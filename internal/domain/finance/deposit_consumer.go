package finance

import (
	"context"
	"encoding/json"
	"errors"
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

// DepositConsumer processes deposit events from Kafka
type DepositConsumer struct {
	depositStore store.DepositStore
	accountStore store.AccountStore
	dbTransactor store.DBTransactor
}

// NewDepositConsumer creates a new instance of DepositConsumer
func NewDepositConsumer(
	depositStore store.DepositStore,
	accountStore store.AccountStore,
	dbTransactor store.DBTransactor,
) *DepositConsumer {
	return &DepositConsumer{
		depositStore: depositStore,
		accountStore: accountStore,
		dbTransactor: dbTransactor,
	}
}

// ProcessMessage processes a message from Kafka
// It handles deposit events by creating or updating deposit records
// and updating account balances when deposits are completed
func (c *DepositConsumer) ProcessMessage(ctx context.Context, message []byte) error {
	// Parse the Kafka event
	var event partnerconsumerkafkav1.DepositEvent
	if err := json.Unmarshal(message, &event); err != nil {
		return fmt.Errorf("failed to unmarshal deposit event: %w", err)
	}

	// Basic validation and parsing amount
	amount, err := c.validateEvent(&event)
	if err != nil {
		logger.Error(ctx, "Invalid deposit event", zap.Error(err))
		return err
	}

	// Add trace ID from event's external ID
	ctx = logger.ContextWithTraceID(ctx, event.ExtId)

	logger.Info(ctx, "Received deposit event",
		zap.String("ext_id", event.ExtId),
		zap.String("ext_account_id", event.ExtAccountId),
		zap.String("status", event.Status.String()))

	// Get deposit by external ID
	deposit, err := c.depositStore.GetByExtID(ctx, event.ExtId)
	if err == nil {
		// Deposit found, update it
		return c.handleExistingDeposit(ctx, deposit, &event)
	}
	if !errors.Is(err, entity.ErrNotFound) {
		return fmt.Errorf("failed to get deposit by ext ID %s: %w", event.ExtId, err)
	}

	// Deposit not found, create a new one
	return c.handleNewDeposit(ctx, &event, amount)
}

// validateEvent validates the deposit event and returns the parsed amount
func (c *DepositConsumer) validateEvent(event *partnerconsumerkafkav1.DepositEvent) (decimal.Decimal, error) {
	if event.ExtId == "" {
		return decimal.Zero, fmt.Errorf("ext_id is empty")
	}

	if event.ExtAccountId == "" {
		return decimal.Zero, fmt.Errorf("ext_account_id is empty")
	}

	if event.Amount == nil || event.Amount.Value == "" {
		return decimal.Zero, fmt.Errorf("amount is empty")
	}

	amount, err := decimal.NewFromString(event.Amount.Value)
	if err != nil {
		return decimal.Zero, fmt.Errorf("invalid amount format: %w", err)
	}

	if amount.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, fmt.Errorf("amount must be positive")
	}

	if event.Currency == "" {
		return decimal.Zero, fmt.Errorf("currency is empty")
	}

	return amount, nil
}

// handleNewDeposit handles a deposit event for a new deposit
func (c *DepositConsumer) handleNewDeposit(ctx context.Context, event *partnerconsumerkafkav1.DepositEvent, amount decimal.Decimal) error {
	// First, get the account by external account ID
	account, err := c.accountStore.GetByExtID(ctx, event.ExtAccountId)
	if err != nil {
		return fmt.Errorf("failed to get account by ext ID %s: %w", event.ExtAccountId, err)
	}

	// Update context with user_id
	ctx = logger.ContextWithUserID(ctx, account.UserID)

	deposit := c.mapToDeposit(account.ID, event, amount)

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
			zap.String("ext_id", event.ExtId),
			zap.String("account_id", account.ID.String()),
			zap.Error(err))
		return err
	}

	return nil
}

// mapToDeposit creates a new Deposit entity from Kafka event
func (c *DepositConsumer) mapToDeposit(accountID uuid.UUID, event *partnerconsumerkafkav1.DepositEvent, amount decimal.Decimal) *entity.Deposit {
	now := time.Now()

	return &entity.Deposit{
		ID:        uuid.New(),
		AccountID: accountID,
		Amount:    amount,
		Currency:  event.Currency,
		Status:    convertKafkaDepositStatus(event.Status),
		ExtID:     event.ExtId,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// handleExistingDeposit handles a deposit event for an existing deposit
func (c *DepositConsumer) handleExistingDeposit(ctx context.Context, deposit *entity.Deposit, event *partnerconsumerkafkav1.DepositEvent) error {
	// Convert status from Kafka event
	status := convertKafkaDepositStatus(event.Status)

	// If deposit becomes completed, update account balance
	if status == entity.DepositStatusCompleted && !deposit.IsCompleted() {
		if err := c.updateAccountBalance(ctx, event, deposit); err != nil {
			return fmt.Errorf("failed to update account balance: %w", err)
		}
	}

	// Update deposit status
	if err := c.depositStore.UpdateStatus(ctx, deposit.ID, status); err != nil {
		return fmt.Errorf("failed to update deposit status: %w", err)
	}

	logger.Info(ctx, "Updated deposit status",
		zap.String("id", deposit.ID.String()),
		zap.String("old_status", string(deposit.Status)),
		zap.String("new_status", string(status)))

	deposit.Status = status // Update status locally
	return nil
}

// updateAccountBalance updates the account balance when a deposit is completed
func (c *DepositConsumer) updateAccountBalance(ctx context.Context, event *partnerconsumerkafkav1.DepositEvent, deposit *entity.Deposit) error {
	// Вычисляем разницу для установки точного баланса
	newBalance, err := decimal.NewFromString(event.Amount.Value)
	if err != nil {
		return fmt.Errorf("failed to parse amount: %w", err)
	}

	// Обновляем баланс с вычисленной разницей
	err = c.accountStore.UpdateBalance(ctx, deposit.AccountID, newBalance)
	if err != nil {
		return fmt.Errorf("failed to set exact balance for account %s: %w", deposit.AccountID, err)
	}

	logger.Info(ctx, "Set exact account balance",
		zap.String("account_id", deposit.AccountID.String()),
		// zap.String("old_balance", account.Balance.String()),
		zap.String("new_balance", newBalance.String()),
		zap.String("added_amount", deposit.Amount.String()),
		zap.String("currency", deposit.Currency))

	return nil
}

// convertKafkaDepositStatus converts Kafka deposit status to internal status
func convertKafkaDepositStatus(status partnerconsumerkafkav1.DepositStatus) entity.DepositStatus {
	switch status {
	case partnerconsumerkafkav1.DepositStatus_DEPOSIT_STATUS_PENDING:
		return entity.DepositStatusPending
	case partnerconsumerkafkav1.DepositStatus_DEPOSIT_STATUS_COMPLETED:
		return entity.DepositStatusCompleted
	case partnerconsumerkafkav1.DepositStatus_DEPOSIT_STATUS_FAILED:
		return entity.DepositStatusFailed
	default:
		return entity.DepositStatusPending
	}
}
