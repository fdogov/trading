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

	// Basic validation
	if err := c.validateEvent(&event); err != nil {
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
	return c.handleNewDeposit(ctx, &event)
}

// validateEvent validates the deposit event
func (c *DepositConsumer) validateEvent(event *partnerconsumerkafkav1.DepositEvent) error {
	if event.ExtId == "" {
		return fmt.Errorf("ext_id is empty")
	}

	if event.ExtAccountId == "" {
		return fmt.Errorf("ext_account_id is empty")
	}

	if event.Amount == nil || event.Amount.Value == "" {
		return fmt.Errorf("amount is empty")
	}

	amount, err := decimal.NewFromString(event.Amount.Value)
	if err != nil {
		return fmt.Errorf("invalid amount format: %w", err)
	}

	if amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("amount must be positive")
	}

	if event.Currency == "" {
		return fmt.Errorf("currency is empty")
	}

	return nil
}

// handleNewDeposit handles a deposit event for a new deposit
func (c *DepositConsumer) handleNewDeposit(ctx context.Context, event *partnerconsumerkafkav1.DepositEvent) error {
	// First, get the account by external account ID
	account, err := c.accountStore.GetByExtID(ctx, event.ExtAccountId)
	if err != nil {
		return fmt.Errorf("failed to get account by ext ID %s: %w", event.ExtAccountId, err)
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
			if err = c.updateAccountBalance(txCtx, deposit); err != nil {
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

func (c *DepositConsumer) mapToDeposit(accountID uuid.UUID, event *partnerconsumerkafkav1.DepositEvent) *entity.Deposit {
	amount, _ := decimal.NewFromString(event.Amount.Value) // Ошибка уже обработана ранее
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

	// If status hasn't changed, do nothing
	if deposit.Status == status {
		logger.Info(ctx, "Deposit status hasn't changed, skipping update",
			zap.String("id", deposit.ID.String()),
			zap.String("status", string(status)))
		return nil
	}

	// Update status and account balance if needed
	err := c.dbTransactor.Exec(ctx, func(txCtx context.Context) error {
		// Update deposit status
		if err := c.depositStore.UpdateStatus(txCtx, deposit.ID, status); err != nil {
			return fmt.Errorf("failed to update deposit status: %w", err)
		}

		logger.Info(txCtx, "Updated deposit status",
			zap.String("id", deposit.ID.String()),
			zap.String("old_status", string(deposit.Status)),
			zap.String("new_status", string(status)))

		// If deposit becomes completed, update account balance
		if status == entity.DepositStatusCompleted && !deposit.IsCompleted() {
			deposit.Status = status // Update status locally
			if err := c.updateAccountBalance(txCtx, deposit); err != nil {
				return fmt.Errorf("failed to update account balance: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		logger.Error(ctx, "Failed to update deposit",
			zap.String("id", deposit.ID.String()),
			zap.String("ext_id", deposit.ExtID),
			zap.Error(err))
		return err
	}

	return nil
}

// updateAccountBalance updates the account balance when a deposit is completed
func (c *DepositConsumer) updateAccountBalance(ctx context.Context, deposit *entity.Deposit) error {
	err := c.accountStore.UpdateBalance(ctx, deposit.AccountID, deposit.Amount.String())
	if err != nil {
		return fmt.Errorf("failed to update account balance for account %s: %w", deposit.AccountID, err)
	}

	logger.Info(ctx, "Updated account balance",
		zap.String("account_id", deposit.AccountID.String()),
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
