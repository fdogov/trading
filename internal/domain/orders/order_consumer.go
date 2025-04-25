package orders

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	partnerconsumerkafkav1 "github.com/fdogov/contracts/gen/go/backend/partnerconsumer/kafka/v1"
	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/logger"
	"github.com/fdogov/trading/internal/store"
)

// OrderConsumer processes order events from Kafka
type OrderConsumer struct {
	orderStore   store.OrderStore
	accountStore store.AccountStore
	dbTransactor store.DBTransactor
}

// NewOrderConsumer creates a new OrderConsumer instance
func NewOrderConsumer(
	orderStore store.OrderStore,
	accountStore store.AccountStore,
	dbTransactor store.DBTransactor,
) *OrderConsumer {
	return &OrderConsumer{
		orderStore:   orderStore,
		accountStore: accountStore,
		dbTransactor: dbTransactor,
	}
}

// ProcessMessage processes a message from Kafka
func (c *OrderConsumer) ProcessMessage(ctx context.Context, message []byte) error {
	var event partnerconsumerkafkav1.OrderEvent
	if err := json.Unmarshal(message, &event); err != nil {
		return fmt.Errorf("failed to unmarshal order event: %w", err)
	}

	// Add trace ID from the event's external ID
	ctx = logger.ContextWithTraceID(ctx, event.ExtId)

	logger.Info(ctx, "Received order event",
		zap.String("ext_account_id", event.ExtAccountId),
		zap.String("status", event.Status.String()),
		zap.String("symbol", event.Symbol))

	// Get the order by external ID
	order, err := c.orderStore.GetByExtID(ctx, event.ExtId)
	if err != nil {
		if err != entity.ErrNotFound {
			return fmt.Errorf("failed to get order by ext ID: %w", err)
		}

		// If the order is not found, it might have been created outside our system
		// Create an order record in our system

		// First get the account by external account ID
		account, err := c.accountStore.GetByExtID(ctx, event.ExtAccountId)
		if err != nil {
			return fmt.Errorf("failed to get account by ext ID: %w", err)
		}

		// Update context with user_id
		ctx = logger.ContextWithUserID(ctx, account.UserID)

		// Convert status
		var status entity.OrderStatus
		switch event.Status {
		case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_PENDING:
			status = entity.OrderStatusProcessing
		case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_COMPLETED:
			status = entity.OrderStatusCompleted
		case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_FAILED:
			status = entity.OrderStatusFailed
		default:
			status = entity.OrderStatusNew
		}

		// Convert amount
		amount, err := decimal.NewFromString(event.Amount.Value)
		if err != nil {
			return fmt.Errorf("failed to parse amount: %w", err)
		}

		// Create order
		now := time.Now()
		order = &entity.Order{
			ID:           uuid.New(),
			UserID:       account.UserID,
			AccountID:    account.ID,
			InstrumentID: event.Symbol,
			Amount:       amount,
			Quantity:     decimal.Zero,           // We don't know the exact quantity yet
			OrderType:    entity.OrderTypeMarket, // Assume it's a market order
			Side:         entity.OrderSideBuy,    // Assume side (can be refined later)
			Status:       status,
			Description:  fmt.Sprintf("Order for %s (ext_id: %s)", event.Symbol, event.ExtId),
			ExtID:        event.ExtId,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		err = c.orderStore.Create(ctx, order)
		if err != nil {
			return fmt.Errorf("failed to create order: %w", err)
		}

		logger.Info(ctx, "Created new order record",
			zap.String("id", order.ID.String()),
			zap.String("account_id", order.AccountID.String()),
			zap.String("instrument", order.InstrumentID),
			zap.String("amount", order.Amount.String()),
			zap.String("status", string(order.Status)))

		return nil
	}

	// If the order is found, update its status

	// Convert status from the event
	var status entity.OrderStatus
	switch event.Status {
	case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_PENDING:
		status = entity.OrderStatusProcessing
	case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_COMPLETED:
		status = entity.OrderStatusCompleted
	case partnerconsumerkafkav1.OrderStatus_ORDER_STATUS_FAILED:
		status = entity.OrderStatusFailed
	default:
		status = entity.OrderStatusNew
	}

	// If status hasn't changed, do nothing
	if order.Status == status {
		logger.Info(ctx, "Order status hasn't changed, skipping update",
			zap.String("status", string(status)))
		return nil
	}

	// Update status and process fund return if necessary
	err = c.dbTransactor.Exec(ctx, func(txCtx context.Context) error {
		// Update order status
		err := c.orderStore.UpdateStatus(txCtx, order.ID, status)
		if err != nil {
			return fmt.Errorf("failed to update order status: %w", err)
		}

		logger.Info(txCtx, "Updated order status",
			zap.String("id", order.ID.String()),
			zap.String("old_status", string(order.Status)),
			zap.String("new_status", string(status)))

		// If the order failed but was a buy, return funds to account
		if status == entity.OrderStatusFailed && order.Status != entity.OrderStatusFailed && order.IsBuy() {
			err := c.accountStore.UpdateBalance(txCtx, order.AccountID, order.Amount)
			if err != nil {
				return fmt.Errorf("failed to return funds to account: %w", err)
			}

			logger.Info(txCtx, "Returned funds to account",
				zap.String("account_id", order.AccountID.String()),
				zap.String("amount", order.Amount.String()))
		}

		return nil
	})

	return err
}
