package finance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fdogov/trading/internal/producers"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	partnerconsumerkafkav1 "github.com/fdogov/contracts/gen/go/backend/partnerconsumer/kafka/v1"
	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/logger"
	"github.com/fdogov/trading/internal/store"
)

// ValidatedDepositEvent содержит проверенные и преобразованные поля из события Kafka.
type ValidatedDepositEvent struct {
	ExtID          string
	ExtAccountID   string
	Amount         decimal.Decimal
	Currency       string
	Status         entity.DepositStatus
	NewBalance     decimal.Decimal // Новое поле для баланса
	CreatedAt      time.Time       // Преобразованное время
	IdempotencyKey string          // Ключ идемпотентности
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

func (c *DepositConsumer) saveEvent(ctx context.Context, event *ValidatedDepositEvent) error {
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
	depositEvent *ValidatedDepositEvent,
) error {
	if !deposit.IsCompleted() {
		return nil // Не отправляем уведомление, если депозит не завершен
	}
	// Получаем информацию об аккаунте для получения userID
	account, err := c.accountStore.GetByID(ctx, deposit.AccountID)
	if err != nil {
		logger.Error(ctx, "Failed to get account for notification",
			zap.String("account_id", deposit.AccountID.String()),
			zap.Error(err))
		return nil // Не блокируем обработку из-за проблем с уведомлением
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

func (c *DepositConsumer) unmarshalAndValidateEvent(ctx context.Context, message []byte) (*ValidatedDepositEvent, error) {
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

// validateAndParseEvent проверяет событие пополнения и возвращает структуру с разобранными значениями.
func (c *DepositConsumer) validateAndParseEvent(event *partnerconsumerkafkav1.DepositEvent) (*ValidatedDepositEvent, error) {
	validated := new(ValidatedDepositEvent)
	var err error

	if event == nil {
		return validated, fmt.Errorf("событие равно nil")
	}

	// Проверка обязательных строковых полей
	if event.ExtId == "" {
		return validated, fmt.Errorf("ext_id пуст")
	}
	validated.ExtID = event.ExtId

	if event.ExtAccountId == "" {
		return validated, fmt.Errorf("ext_account_id пуст")
	}
	validated.ExtAccountID = event.ExtAccountId

	if event.Currency == "" {
		return validated, fmt.Errorf("currency пуст")
	}
	validated.Currency = event.Currency

	if event.IdempotencyKey == "" {
		// Предполагаем, что IdempotencyKey обязателен
		return validated, fmt.Errorf("idempotency_key пуст")
	}
	validated.IdempotencyKey = event.IdempotencyKey

	// Проверка и парсинг Amount
	if event.Amount == nil || event.Amount.Value == "" {
		return validated, fmt.Errorf("amount пуст")
	}
	validated.Amount, err = decimal.NewFromString(event.Amount.Value)
	if err != nil {
		return validated, fmt.Errorf("неверный формат amount: %w", err)
	}
	if validated.Amount.LessThanOrEqual(decimal.Zero) {
		return validated, fmt.Errorf("amount должен быть положительным")
	}

	// Проверка и парсинг BalanceNew (опционально, может отсутствовать)
	// Если поле обязательно, добавьте более строгую проверку.
	if event.BalanceNew != nil && event.BalanceNew.Value != "" {
		validated.NewBalance, err = decimal.NewFromString(event.BalanceNew.Value)
		if err != nil {
			// Можно вернуть ошибку или установить значение по умолчанию,
			// в зависимости от требований. Здесь возвращаем ошибку.
			return validated, fmt.Errorf("неверный формат balance_new: %w", err)
		}
	} else {
		// Обработка случая, когда BalanceNew не предоставлен.
		// Устанавливаем ноль по умолчанию, если это допустимо.
		validated.NewBalance = decimal.Zero
	}

	// Проверка и преобразование CreatedAt
	if event.CreatedAt == nil {
		return validated, fmt.Errorf("created_at равно nil")
	}
	if err = event.CreatedAt.CheckValid(); err != nil {
		return validated, fmt.Errorf("неверная временная метка created_at: %w", err)
	}
	validated.CreatedAt = event.CreatedAt.AsTime()

	validated.Status, err = convertKafkaDepositStatus(event.Status)
	if err != nil {
		return validated, fmt.Errorf("неверный статус депозита: %w", err)
	}

	return validated, nil
}

func (c *DepositConsumer) handleDeposit(ctx context.Context, depositEvent *ValidatedDepositEvent) (*entity.Deposit, error) {
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
	event *ValidatedDepositEvent,
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
	event *ValidatedDepositEvent,
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
	event *ValidatedDepositEvent,
	deposit *entity.Deposit,
) error {
	// Обновляем баланс с вычисленной разницей
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
func (c *DepositConsumer) mapToDeposit(accountID uuid.UUID, event *ValidatedDepositEvent) *entity.Deposit {
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
		return entity.DepositStatusUnspecified, fmt.Errorf("неизвестный статус депозита: %s", status.String())
	}
}
