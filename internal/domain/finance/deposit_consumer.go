package finance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// DepositEventData contains validated and transformed fields from the Kafka event.
type DepositEventData struct {
	ExtID          string
	ExtAccountID   string
	Amount         decimal.Decimal
	Currency       string
	Status         entity.DepositStatus
	NewBalance     decimal.Decimal // New field for balance
	CreatedAt      time.Time       // Transformed time
	IdempotencyKey string          // Idempotency key
}

// DepositConsumer processes deposit events from Kafka
type DepositConsumer struct {
	depositStore    store.DepositStore
	accountStore    store.AccountStore
	dbTransactor    store.DBTransactor
	eventStore      store.EventStore
	depositProducer producers.DepositProducerI
}

// NewDepositConsumer creates a new instance of DepositConsumer
func NewDepositConsumer(
	depositStore store.DepositStore,
	accountStore store.AccountStore,
	eventStore store.EventStore,
	depositProducer producers.DepositProducerI,
	dbTransactor store.DBTransactor,
) *DepositConsumer {
	return &DepositConsumer{
		depositStore:    depositStore,
		accountStore:    accountStore,
		eventStore:      eventStore,
		depositProducer: depositProducer,
		dbTransactor:    dbTransactor,
	}
}

// ProcessMessage processes a message from Kafka
// It handles deposit events by creating or updating deposit records
// and updating account balances when deposits are completed
func (c *DepositConsumer) ProcessMessage(ctx context.Context, message []byte) error {
	// Basic validation and parsing amount
	depositEvent, err := c.unmarshalAndValidateEvent(ctx, message)
	if err != nil {
		return err
	}

	_, err = c.eventStore.GetByEventID(ctx, depositEvent.IdempotencyKey, entity.EventTypeDeposit)
	if err == nil {
		logger.Info(ctx, "Event already processed",
			zap.String("idempotency_key", depositEvent.IdempotencyKey))
		return nil // Event already processed, skip
	}

	// Add trace ID from event's external ID
	ctx = logger.ContextWithTraceID(ctx, depositEvent.ExtID)

	logger.Info(ctx, "Received deposit event",
		zap.String("ext_id", depositEvent.ExtID),
		zap.String("ext_account_id", depositEvent.ExtAccountID),
		zap.String("status", depositEvent.Status.String()))

	// Get deposit by external ID
	deposit, err := c.handleDeposit(ctx, depositEvent)
	if err != nil {
		return fmt.Errorf("failed to handle deposit: %w", err)
	}

	if err = c.sendToOperationFeed(ctx, deposit, depositEvent); err != nil {
		return fmt.Errorf("failed to send deposit notification: %w", err)
	}

	return c.saveEvent(ctx, depositEvent)
}

func (c *DepositConsumer) saveEvent(ctx context.Context, event *DepositEventData) error {
	// Save event for idempotency
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

func (c *DepositConsumer) sendToOperationFeed(
	ctx context.Context,
	deposit *entity.Deposit,
	depositEvent *DepositEventData,
) error {
	if !deposit.IsCompleted() {
		return nil // Do not send notification if deposit is not completed
	}
	// Get account information to get userID
	account, err := c.accountStore.GetByID(ctx, deposit.AccountID)
	if err != nil {
		logger.Error(ctx, "Failed to get account for notification",
			zap.String("account_id", deposit.AccountID.String()),
			zap.Error(err))
		return nil // Do not block processing due to notification issues
	}

	err = c.depositProducer.SendDepositEvent(
		ctx,
		deposit,
		account.UserID,
		depositEvent.NewBalance,
		depositEvent.IdempotencyKey,
	)
	if err != nil {
		logger.Error(ctx, "Failed to send deposit notification",
			zap.String("id", deposit.ID.String()),
			zap.Error(err))
		return fmt.Errorf("failed to send deposit notification: %w", err)
	}
	return nil
}

func (c *DepositConsumer) unmarshalAndValidateEvent(ctx context.Context, message []byte) (*DepositEventData, error) {
	// Parse the Kafka event
	var event partnerconsumerkafkav1.DepositEvent
	if err := json.Unmarshal(message, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal deposit event: %w", err)
	}

	// Basic validation and parsing amount
	depositEvent, err := c.validateAndParseEvent(&event)
	if err != nil {
		logger.Error(ctx, "Invalid deposit event", zap.Error(err))
		return nil, err
	}
	return depositEvent, nil
}

// validateAndParseEvent validates the deposit event and returns a structure with parsed values.
func (c *DepositConsumer) validateAndParseEvent(event *partnerconsumerkafkav1.DepositEvent) (*DepositEventData, error) {
	validated := new(DepositEventData)
	var err error

	if event == nil {
		return validated, fmt.Errorf("event is nil")
	}

	// Checking required string fields
	if event.ExtId == "" {
		return validated, fmt.Errorf("ext_id is empty")
	}
	validated.ExtID = event.ExtId

	if event.ExtAccountId == "" {
		return validated, fmt.Errorf("ext_account_id is empty")
	}
	validated.ExtAccountID = event.ExtAccountId

	if event.Currency == "" {
		return validated, fmt.Errorf("currency is empty")
	}
	validated.Currency = event.Currency

	if event.IdempotencyKey == "" {
		// Assuming IdempotencyKey is required
		return validated, fmt.Errorf("idempotency_key is empty")
	}
	validated.IdempotencyKey = event.IdempotencyKey

	// Checking and parsing Amount
	if event.Amount == nil || event.Amount.Value == "" {
		return validated, fmt.Errorf("amount is empty")
	}
	validated.Amount, err = decimal.NewFromString(event.Amount.Value)
	if err != nil {
		return validated, fmt.Errorf("invalid amount format: %w", err)
	}
	if validated.Amount.LessThanOrEqual(decimal.Zero) {
		return validated, fmt.Errorf("amount must be positive")
	}

	// Checking and parsing BalanceNew (optional, may be missing)
	// If the field is required, add more strict validation.
	if event.BalanceNew != nil && event.BalanceNew.Value != "" {
		validated.NewBalance, err = decimal.NewFromString(event.BalanceNew.Value)
		if err != nil {
			// Can return an error or set a default value,
			// depending on requirements. Here we return an error.
			return validated, fmt.Errorf("invalid balance_new format: %w", err)
		}
	} else {
		// Handling the case when BalanceNew is not provided.
		// Set zero by default if allowed.
		validated.NewBalance = decimal.Zero
	}

	// Checking and converting CreatedAt
	if event.CreatedAt == nil {
		return validated, fmt.Errorf("created_at is nil")
	}
	if err = event.CreatedAt.CheckValid(); err != nil {
		return validated, fmt.Errorf("invalid created_at timestamp: %w", err)
	}
	validated.CreatedAt = event.CreatedAt.AsTime()

	validated.Status, err = convertKafkaDepositStatus(event.Status)
	if err != nil {
		return validated, fmt.Errorf("invalid deposit status: %w", err)
	}

	return validated, nil
}

func (c *DepositConsumer) handleDeposit(ctx context.Context, depositEvent *DepositEventData) (*entity.Deposit, error) {
	// Get deposit by external ID
	deposit, err := c.depositStore.GetByExtID(ctx, depositEvent.ExtID)
	if err == nil {
		// Deposit found, update it
		return c.handleExistingDeposit(ctx, deposit, depositEvent)
	}
	if !errors.Is(err, entity.ErrNotFound) {
		return nil, fmt.Errorf("failed to get deposit by ext ID %s: %w", depositEvent.ExtID, err)
	}

	// Deposit not found, create a new one
	return c.handleNewDeposit(ctx, depositEvent)
}

// handleNewDeposit handles a deposit event for a new deposit
func (c *DepositConsumer) handleNewDeposit(
	ctx context.Context,
	event *DepositEventData,
) (*entity.Deposit, error) {
	// First, get the account by external account ID
	account, err := c.accountStore.GetByExtID(ctx, event.ExtAccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account by ext ID %s: %w", event.ExtAccountID, err)
	}

	// Update context with user_id
	ctx = logger.ContextWithUserID(ctx, account.UserID)

	deposit := c.mapToDeposit(account.ID, event)

	// Use transaction to create deposit and update balance if needed
	err = c.dbTransactor.Exec(ctx, func(txCtx context.Context) error {
		// Create the deposit
		if err = c.depositStore.Create(txCtx, deposit); err != nil {
			return fmt.Errorf("failed to create deposit: %w", err)
		}

		logger.Info(txCtx, "Created new deposit record",
			zap.String("id", deposit.ID.String()),
			zap.String("account_id", deposit.AccountID.String()),
			zap.String("amount", deposit.Amount.String()),
			zap.String("currency", deposit.Currency),
			zap.String("status", string(deposit.Status)))

		// If deposit is completed, update account balance
		if deposit.IsCompleted() {
			if err = c.updateAccountBalance(txCtx, event, deposit); err != nil {
				return fmt.Errorf("failed to update account balance: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		logger.Error(ctx, "Failed to process new deposit",
			zap.String("ext_id", event.ExtID),
			zap.String("account_id", account.ID.String()),
			zap.Error(err))
		return nil, err
	}

	return deposit, nil
}

// handleExistingDeposit handles a deposit event for an existing deposit
func (c *DepositConsumer) handleExistingDeposit(
	ctx context.Context,
	deposit *entity.Deposit,
	event *DepositEventData,
) (*entity.Deposit, error) {

	// If deposit becomes completed, update account balance
	if event.Status == entity.DepositStatusCompleted && !deposit.IsCompleted() {
		if err := c.updateAccountBalance(ctx, event, deposit); err != nil {
			return nil, fmt.Errorf("failed to update account balance: %w", err)
		}
	}

	// Update deposit status
	if err := c.depositStore.UpdateStatus(ctx, deposit.ID, event.Status); err != nil {
		return nil, fmt.Errorf("failed to update deposit status: %w", err)
	}

	logger.Info(ctx, "Updated deposit status",
		zap.String("id", deposit.ID.String()),
		zap.String("old_status", deposit.Status.String()),
		zap.String("new_status", event.Status.String()))

	deposit.Status = event.Status // Update status locally
	return deposit, nil
}

// updateAccountBalance updates the account balance when a deposit is completed
func (c *DepositConsumer) updateAccountBalance(
	ctx context.Context,
	event *DepositEventData,
	deposit *entity.Deposit,
) error {
	// Update balance with calculated difference
	if err := c.accountStore.UpdateBalance(ctx, deposit.AccountID, event.NewBalance); err != nil {
		return fmt.Errorf("failed to set exact balance for account %s: %w", deposit.AccountID, err)
	}

	logger.Info(ctx, "Set exact account balance",
		zap.String("account_id", deposit.AccountID.String()),
		// zap.String("old_balance", account.Balance.String()),
		zap.String("new_balance", event.NewBalance.String()),
		zap.String("added_amount", deposit.Amount.String()),
		zap.String("currency", deposit.Currency))

	return nil
}

// mapToDeposit creates a new Deposit entity from Kafka event
func (c *DepositConsumer) mapToDeposit(accountID uuid.UUID, event *DepositEventData) *entity.Deposit {
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
