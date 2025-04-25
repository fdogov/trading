package finance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fdogov/trading/internal/dependency"
	"time"

	"github.com/fdogov/trading/internal/producers"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	partnerconsumerkafkav1 "github.com/fdogov/contracts/gen/go/backend/partnerconsumer/kafka/v1"
	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/logger"
	"github.com/fdogov/trading/internal/store"
)

// DepositEventData contains validated and transformed fields from Kafka event.
type DepositEventData struct {
	ExtID          string
	ExtAccountID   string
	Amount         decimal.Decimal
	Currency       string
	Status         entity.DepositStatus
	NewBalance     decimal.Decimal
	CreatedAt      time.Time
	IdempotencyKey string
}

// DepositConsumer processes deposit events from Kafka
type DepositConsumer struct {
	depositStore              store.DepositStore
	accountStore              store.AccountStore
	eventStore                store.EventStore
	depositProducer           producers.DepositProducerI
	partnerProxyAccountClient dependency.PartnerProxyAccountClient
}

// NewDepositConsumer creates a new instance of DepositConsumer
func NewDepositConsumer(
	depositStore store.DepositStore,
	accountStore store.AccountStore,
	eventStore store.EventStore,
	depositProducer producers.DepositProducerI,
	partnerProxyAccountClient dependency.PartnerProxyAccountClient,
) *DepositConsumer {
	return &DepositConsumer{
		depositStore:              depositStore,
		accountStore:              accountStore,
		eventStore:                eventStore,
		depositProducer:           depositProducer,
		partnerProxyAccountClient: partnerProxyAccountClient,
	}
}

// ProcessMessage processes a message from Kafka
// It handles deposit events by creating or updating deposit records
// and updating account balances when deposits are completed
func (c *DepositConsumer) ProcessMessage(ctx context.Context, message []byte) error {
	// Parsing and validating the event
	depositEvent, err := c.unmarshalAndValidateEvent(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to validate deposit event: %w", err)
	}

	// Checking idempotency
	if processed, err := c.isEventAlreadyProcessed(ctx, depositEvent.IdempotencyKey); err != nil {
		return fmt.Errorf("failed to check idempotency: %w", err)
	} else if processed {
		return nil // Event already processed, skipping
	}

	// Adding trace ID from event's external ID
	ctx = logger.ContextWithTraceID(ctx, depositEvent.ExtID)

	logger.Info(ctx, "Processing deposit event",
		zap.String("ext_id", depositEvent.ExtID),
		zap.String("ext_account_id", depositEvent.ExtAccountID),
		zap.String("status", depositEvent.Status.String()),
		zap.String("amount", depositEvent.Amount.String()),
		zap.String("currency", depositEvent.Currency))

	// Processing deposit (create or update)
	deposit, err := c.handleDeposit(ctx, depositEvent)
	if err != nil {
		return fmt.Errorf("failed to handle deposit: %w", err)
	}

	// Updating account balance if deposit is completed
	if err = c.updateAccountBalance(ctx, depositEvent, deposit); err != nil {
		return fmt.Errorf("failed to update account balance: %w", err)
	}

	// Sending deposit notification
	if err = c.notifyDepositCompletion(ctx, deposit, depositEvent); err != nil {
		return fmt.Errorf("failed to send deposit notification: %w", err)
	}

	// Recording event processing for idempotency
	return c.recordEventProcessing(ctx, depositEvent)
}

// isEventAlreadyProcessed checks if the event has already been processed
func (c *DepositConsumer) isEventAlreadyProcessed(ctx context.Context, idempotencyKey string) (bool, error) {
	_, err := c.eventStore.GetByEventID(ctx, idempotencyKey, entity.EventTypeDeposit)
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
func (c *DepositConsumer) recordEventProcessing(ctx context.Context, event *DepositEventData) error {
	eventEntity := entity.NewEvent(
		entity.EventTypeDeposit,
		event.IdempotencyKey,
		event.CreatedAt,
	)
	if err := c.eventStore.Create(ctx, eventEntity); err != nil {
		return fmt.Errorf("failed to create event record: %w", err)
	}
	return nil
}

// notifyDepositCompletion sends a notification about deposit completion
func (c *DepositConsumer) notifyDepositCompletion(
	ctx context.Context,
	deposit *entity.Deposit,
	depositEvent *DepositEventData,
) error {
	// Only send notifications for completed deposits
	if !deposit.IsCompleted() {
		return nil
	}

	// Get account information to retrieve userID
	account, err := c.accountStore.GetByID(ctx, deposit.AccountID)
	if err != nil {
		logger.Error(ctx, "Failed to get account for notification",
			zap.String("account_id", deposit.AccountID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to get account: %w", err)
	}

	// Send deposit notification
	if err = c.depositProducer.SendDepositEvent(
		ctx,
		deposit,
		account.UserID,
		depositEvent.NewBalance,
		depositEvent.IdempotencyKey,
	); err != nil {
		logger.Error(ctx, "Failed to send deposit notification",
			zap.String("deposit_id", deposit.ID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to send deposit notification: %w", err)
	}

	logger.Info(ctx, "Sent deposit completion notification",
		zap.String("deposit_id", deposit.ID.String()),
		zap.String("user_id", account.UserID),
		zap.String("amount", deposit.Amount.String()),
		zap.String("currency", deposit.Currency))

	return nil
}

// unmarshalAndValidateEvent parses and validates raw Kafka message
func (c *DepositConsumer) unmarshalAndValidateEvent(ctx context.Context, message []byte) (*DepositEventData, error) {
	var event partnerconsumerkafkav1.DepositEvent
	if err := json.Unmarshal(message, &event); err != nil {
		logger.Error(ctx, "Failed to unmarshal deposit event", zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal deposit event: %w", err)
	}

	// Validating and transforming event data
	depositEvent, err := c.validateAndTransformEvent(&event)
	if err != nil {
		logger.Error(ctx, "Invalid deposit event", zap.Error(err))
		return nil, err
	}

	return depositEvent, nil
}

// validateAndTransformEvent validates the deposit event and returns a structure with transformed values
func (c *DepositConsumer) validateAndTransformEvent(event *partnerconsumerkafkav1.DepositEvent) (*DepositEventData, error) {
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
	if event.Currency == "" {
		return nil, errors.New("currency is empty")
	}
	if event.IdempotencyKey == "" {
		return nil, errors.New("idempotency_key is empty")
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

	// Check and parse BalanceNew (optional)
	var newBalance decimal.Decimal
	if event.BalanceNew != nil && event.BalanceNew.Value != "" {
		newBalance, err = decimal.NewFromString(event.BalanceNew.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid balance_new format: %w", err)
		}
	} else {
		newBalance = decimal.Zero
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
	status, err := convertKafkaDepositStatus(event.Status)
	if err != nil {
		return nil, fmt.Errorf("invalid deposit status: %w", err)
	}

	// Return validated data
	return &DepositEventData{
		ExtID:          event.ExtId,
		ExtAccountID:   event.ExtAccountId,
		Amount:         amount,
		Currency:       event.Currency,
		Status:         status,
		NewBalance:     newBalance,
		CreatedAt:      createdAt,
		IdempotencyKey: event.IdempotencyKey,
	}, nil
}

// handleDeposit processes deposit event, creating a new deposit or updating an existing one
func (c *DepositConsumer) handleDeposit(ctx context.Context, depositEvent *DepositEventData) (*entity.Deposit, error) {
	// Try to find existing deposit
	deposit, err := c.depositStore.GetByExtID(ctx, depositEvent.ExtID)
	if err == nil {
		// Deposit found, updating it
		return c.updateExistingDeposit(ctx, deposit, depositEvent)
	}

	// Check that the error is "not found"
	if !errors.Is(err, entity.ErrNotFound) {
		return nil, fmt.Errorf("error retrieving deposit by ext ID %s: %w", depositEvent.ExtID, err)
	}

	// Deposit not found, creating a new one
	return c.createNewDeposit(ctx, depositEvent)
}

// createNewDeposit creates a new deposit record and updates account balance if necessary
func (c *DepositConsumer) createNewDeposit(
	ctx context.Context,
	event *DepositEventData,
) (*entity.Deposit, error) {
	// Find account by external ID
	account, err := c.accountStore.GetByExtID(ctx, event.ExtAccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to find account by ext ID %s: %w", event.ExtAccountID, err)
	}

	// Add user_id to context for better logging
	ctx = logger.ContextWithUserID(ctx, account.UserID)

	// Create deposit entity
	deposit := newDepositFromEvent(account.ID, event)

	if err = c.depositStore.Create(ctx, deposit); err != nil {
		return nil, fmt.Errorf("failed to create deposit: %w", err)
	}

	logger.Info(ctx, "Created new deposit",
		zap.String("id", deposit.ID.String()),
		zap.String("account_id", deposit.AccountID.String()),
		zap.String("amount", deposit.Amount.String()),
		zap.String("currency", deposit.Currency),
		zap.String("status", deposit.Status.String()))

	return deposit, nil
}

// updateExistingDeposit updates an existing deposit and handles balance updates if necessary
func (c *DepositConsumer) updateExistingDeposit(
	ctx context.Context,
	deposit *entity.Deposit,
	event *DepositEventData,
) (*entity.Deposit, error) {
	// Update deposit status
	prevStatus := deposit.Status
	if err := c.depositStore.UpdateStatus(ctx, deposit.ID, event.Status); err != nil {
		return nil, fmt.Errorf("failed to update deposit status: %w", err)
	}

	logger.Info(ctx, "Updated deposit status",
		zap.String("id", deposit.ID.String()),
		zap.String("old_status", prevStatus.String()),
		zap.String("new_status", event.Status.String()))

	// Update local deposit object for consistent return
	deposit.Status = event.Status
	deposit.UpdatedAt = time.Now()

	return deposit, nil
}

// updateAccountBalance updates the account balance when a deposit is completed
func (c *DepositConsumer) updateAccountBalance(
	ctx context.Context,
	event *DepositEventData,
	deposit *entity.Deposit,
) error {
	if !deposit.IsCompleted() {
		// No balance update needed for non-completed deposits
		return nil
	}
	newBalance, err := c.partnerProxyAccountClient.GetAccountBalance(ctx, event.ExtAccountID)
	if err != nil {
		return fmt.Errorf("failed to get account balance: %w", err)
	}

	// Update balance with exact new value
	if err = c.accountStore.UpdateBalance(ctx, deposit.AccountID, *newBalance); err != nil {
		return fmt.Errorf("failed to update balance for account %s: %w", deposit.AccountID, err)
	}

	logger.Info(ctx, "Updated account balance",
		zap.String("account_id", deposit.AccountID.String()),
		zap.String("new_balance", event.NewBalance.String()),
		zap.String("deposit_amount", deposit.Amount.String()),
		zap.String("currency", deposit.Currency))

	return nil
}

// newDepositFromEvent creates a new Deposit entity from validated event
func newDepositFromEvent(accountID uuid.UUID, event *DepositEventData) *entity.Deposit {
	now := time.Now()

	return &entity.Deposit{
		ID:        uuid.New(),
		AccountID: accountID,
		Amount:    event.Amount,
		Currency:  event.Currency,
		Status:    event.Status,
		ExtID:     event.ExtID,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// convertKafkaDepositStatus converts Kafka deposit status to internal status
func convertKafkaDepositStatus(status partnerconsumerkafkav1.DepositStatus) (entity.DepositStatus, error) {
	switch status {
	case partnerconsumerkafkav1.DepositStatus_DEPOSIT_STATUS_PENDING:
		return entity.DepositStatusPending, nil
	case partnerconsumerkafkav1.DepositStatus_DEPOSIT_STATUS_COMPLETED:
		return entity.DepositStatusCompleted, nil
	case partnerconsumerkafkav1.DepositStatus_DEPOSIT_STATUS_FAILED:
		return entity.DepositStatusFailed, nil
	default:
		return entity.DepositStatusUnspecified, fmt.Errorf("unknown deposit status: %s", status.String())
	}
}
