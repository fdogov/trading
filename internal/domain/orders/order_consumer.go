package orders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	partnerconsumerkafkav1 "github.com/fdogov/contracts/gen/go/backend/partnerconsumer/kafka/v1"
	"github.com/fdogov/trading/internal/dependency"
	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/logger"
	"github.com/fdogov/trading/internal/producers"
	"github.com/fdogov/trading/internal/store"
)

// OrderEventData contains validated and transformed fields from Kafka event
type OrderEventData struct {
	ExtID          string
	ExtAccountID   string
	Symbol         string
	Amount         decimal.Decimal
	Status         entity.OrderStatus
	CreatedAt      time.Time
	IdempotencyKey string
}

// OrderConsumer processes order events from Kafka
type OrderConsumer struct {
	orderStore                store.OrderStore
	accountStore              store.AccountStore
	eventStore                store.EventStore
	orderProducer             producers.OrderProducerI
	partnerProxyAccountClient dependency.PartnerProxyAccountClient
}

// NewOrderConsumer creates a new OrderConsumer instance
func NewOrderConsumer(
	orderStore store.OrderStore,
	accountStore store.AccountStore,
	eventStore store.EventStore,
	orderProducer producers.OrderProducerI,
	partnerProxyAccountClient dependency.PartnerProxyAccountClient,
) *OrderConsumer {
	return &OrderConsumer{
		orderStore:                orderStore,
		accountStore:              accountStore,
		eventStore:                eventStore,
		orderProducer:             orderProducer,
		partnerProxyAccountClient: partnerProxyAccountClient,
	}
}

// ProcessMessage processes a message from Kafka
// It handles order events by creating or updating order records
// and updating account balances when orders are failed (returning funds)
func (c *OrderConsumer) ProcessMessage(ctx context.Context, message []byte) error {
	// Parsing and validating the event
	orderEvent, err := c.unmarshalAndValidateEvent(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to validate order event: %w", err)
	}

	// Checking idempotency
	if processed, err := c.isEventAlreadyProcessed(ctx, orderEvent.IdempotencyKey); err != nil {
		return fmt.Errorf("failed to check idempotency: %w", err)
	} else if processed {
		return nil // Event already processed, skipping
	}

	// Adding trace ID from event's external ID
	ctx = logger.ContextWithTraceID(ctx, orderEvent.ExtID)

	logger.Info(ctx, "Processing order event",
		zap.String("ext_id", orderEvent.ExtID),
		zap.String("ext_account_id", orderEvent.ExtAccountID),
		zap.String("status", string(orderEvent.Status)),
		zap.String("symbol", orderEvent.Symbol),
		zap.String("idempotency_key", orderEvent.IdempotencyKey))

	// Processing order (update only)
	if err = c.handleOrder(ctx, orderEvent); err != nil {
		return fmt.Errorf("failed to handle order: %w", err)
	}

	// Recording event processing for idempotency
	return c.recordEventProcessing(ctx, orderEvent)
}

// isEventAlreadyProcessed checks if the event has already been processed
func (c *OrderConsumer) isEventAlreadyProcessed(ctx context.Context, idempotencyKey string) (bool, error) {
	_, err := c.eventStore.GetByEventID(ctx, idempotencyKey, entity.EventTypeOrder)
	if err == nil {
		logger.Info(ctx, "Event already processed", zap.String("idempotency_key", idempotencyKey))
		return true, nil
	}

	if errors.Is(err, entity.ErrNotFound) {
		return false, nil
	}

	return false, fmt.Errorf("error checking event status: %w", err)
}

// recordEventProcessing records the fact that the event has been processed to ensure idempotency
func (c *OrderConsumer) recordEventProcessing(ctx context.Context, event *OrderEventData) error {
	eventEntity := entity.NewEvent(
		entity.EventTypeOrder,
		event.IdempotencyKey,
		event.CreatedAt,
	)
	if err := c.eventStore.Create(ctx, eventEntity); err != nil {
		return fmt.Errorf("failed to create event record: %w", err)
	}
	return nil
}

// unmarshalAndValidateEvent parses and validates raw Kafka message
func (c *OrderConsumer) unmarshalAndValidateEvent(ctx context.Context, message []byte) (*OrderEventData, error) {
	var event partnerconsumerkafkav1.OrderEvent
	if err := json.Unmarshal(message, &event); err != nil {
		logger.Error(ctx, "Failed to unmarshal order event", zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal order event: %w", err)
	}

	// Validating and transforming event data
	orderEvent, err := c.validateAndTransformEvent(&event)
	if err != nil {
		logger.Error(ctx, "Invalid order event", zap.Error(err))
		return nil, err
	}

	return orderEvent, nil
}

// validateAndTransformEvent validates the order event and returns a structure with transformed values
func (c *OrderConsumer) validateAndTransformEvent(event *partnerconsumerkafkav1.OrderEvent) (*OrderEventData, error) {
	if event == nil {
		return nil, errors.New("event is nil")
	}

	// Check required string fields
	if event.ExtId == "" {
		return nil, errors.New("ext_id is empty")
	}
	if event.ExtAccountId == "" {
		return nil, errors.New("ext_account_id is empty")
	}
	if event.Symbol == "" {
		return nil, errors.New("symbol is empty")
	}

	// Check and parse Amount
	if event.Amount == nil || event.Amount.Value == "" {
		return nil, errors.New("amount is empty")
	}
	amount, err := decimal.NewFromString(event.Amount.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid amount format: %w", err)
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, errors.New("amount must be positive")
	}

	// Check and transform CreatedAt
	if event.CreatedAt == nil {
		return nil, errors.New("created_at is nil")
	}
	if err = event.CreatedAt.CheckValid(); err != nil {
		return nil, fmt.Errorf("invalid created_at timestamp: %w", err)
	}
	createdAt := event.CreatedAt.AsTime()

	// Convert status
	status, err := convertKafkaOrderStatus(event.Status)
	if err != nil {
		return nil, fmt.Errorf("invalid order status: %w", err)
	}

	// Check for idempotency_key in the event
	if event.IdempotencyKey == "" {
		return nil, errors.New("idempotency_key is empty")
	}
	idempotencyKey := event.IdempotencyKey

	// Return validated data
	return &OrderEventData{
		ExtID:          event.ExtId,
		ExtAccountID:   event.ExtAccountId,
		Symbol:         event.Symbol,
		Amount:         amount,
		Status:         status,
		CreatedAt:      createdAt,
		IdempotencyKey: idempotencyKey,
	}, nil
}

// handleOrder processes order event, updating an existing order
// It returns an error if the order does not exist, as orders should be created through API
func (c *OrderConsumer) handleOrder(ctx context.Context, orderEvent *OrderEventData) error {
	// If not found by idempotency key, try to find by external ID
	order, err := c.orderStore.GetByExtID(ctx, orderEvent.ExtID)
	if err == nil {
		// Order found by external ID, updating it
		logger.Info(ctx, "Found order by external ID",
			zap.String("ext_id", orderEvent.ExtID),
			zap.String("order_id", order.ID.String()))
		return c.updateExistingOrder(ctx, order, orderEvent)
	}

	// Check that the error is "not found"
	if !errors.Is(err, entity.ErrNotFound) {
		return fmt.Errorf("error retrieving order by ext ID %s: %w", orderEvent.ExtID, err)
	}

	// Order not found, returning an error
	logger.Error(ctx, "Received event for non-existent order",
		zap.String("ext_id", orderEvent.ExtID),
		zap.String("idempotency_key", orderEvent.IdempotencyKey))
	return fmt.Errorf("order with ext_id %s and idempotency_key %s not found",
		orderEvent.ExtID, orderEvent.IdempotencyKey)
}

// updateExistingOrder updates an existing order
func (c *OrderConsumer) updateExistingOrder(
	ctx context.Context,
	order *entity.Order,
	event *OrderEventData,
) error {
	// Update order status
	prevStatus := order.Status
	if err := c.orderStore.UpdateStatus(ctx, order.ID, event.Status); err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	logger.Info(ctx, "Updated order status",
		zap.String("id", order.ID.String()),
		zap.String("old_status", string(prevStatus)),
		zap.String("new_status", string(event.Status)))

	// Update local order object for consistent return
	order.Status = event.Status
	order.UpdatedAt = time.Now()

	// Update account balance
	newBalance, err := c.updateAccountBalance(ctx, order, event)
	if err != nil {
		return fmt.Errorf("failed to update account balance: %w", err)
	}

	// Send order event to Kafka with the updated balance
	if err := c.orderProducer.SendOrderEvent(ctx, order, *newBalance); err != nil {
		// Log the error but don't fail the whole operation
		logger.Error(ctx, "Failed to send order event to Kafka",
			zap.String("order_id", order.ID.String()),
			zap.Error(err))
	} else {
		logger.Info(ctx, "Sent order update to Kafka",
			zap.String("order_id", order.ID.String()),
			zap.String("status", string(order.Status)))
	}

	return nil
}

// updateAccountBalance updates the account balance before sending messages to Kafka
func (c *OrderConsumer) updateAccountBalance(
	ctx context.Context,
	order *entity.Order,
	event *OrderEventData,
) (*decimal.Decimal, error) {
	// Get the latest balance from the partner service
	newBalance, err := c.partnerProxyAccountClient.GetAccountBalance(ctx, event.ExtAccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account balance: %w", err)
	}

	// Update balance with exact new value
	if err = c.accountStore.UpdateBalance(ctx, order.AccountID, *newBalance); err != nil {
		return nil, fmt.Errorf("failed to update balance for account %s: %w", order.AccountID, err)
	}

	logger.Info(ctx, "Updated account balance",
		zap.String("account_id", order.AccountID.String()),
		zap.String("new_balance", newBalance.String()),
		zap.String("order_id", order.ID.String()),
		zap.String("status", string(order.Status)))

	return newBalance, nil
}

// convertKafkaOrderStatus converts Kafka order status to internal status
func convertKafkaOrderStatus(status partnerconsumerkafkav1.OrderStatus) (entity.OrderStatus, error) {
	switch status {
	case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_PENDING:
		return entity.OrderStatusProcessing, nil
	case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_COMPLETED:
		return entity.OrderStatusCompleted, nil
	case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_FAILED:
		return entity.OrderStatusFailed, nil
	case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_CANCELLED:
		return entity.OrderStatusCancelled, nil
	default:
		return entity.OrderStatusUnspecified, fmt.Errorf("unknown order status: %s", status.String())
	}
}
