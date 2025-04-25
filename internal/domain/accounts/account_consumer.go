package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	originationkafkav1 "github.com/fdogov/contracts/gen/go/backend/origination/kafka/v1"
	"github.com/fdogov/trading/internal/entity"
	"github.com/fdogov/trading/internal/store"
)

// ErrInvalidAccountEvent представляет ошибку невалидного события аккаунта
var ErrInvalidAccountEvent = errors.New("invalid account event")

// AccountConsumer обрабатывает события создания аккаунтов из Kafka
type AccountConsumer struct {
	accountStore store.AccountStore
	logger       *zap.Logger
}

// NewAccountConsumer создает новый экземпляр AccountConsumer
func NewAccountConsumer(accountStore store.AccountStore, logger *zap.Logger) *AccountConsumer {
	return &AccountConsumer{
		accountStore: accountStore,
		logger:       logger,
	}
}

// ProcessMessage обрабатывает сообщение из Kafka
func (c *AccountConsumer) ProcessMessage(ctx context.Context, message []byte) error {
	if len(message) == 0 {
		return fmt.Errorf("empty message")
	}

	logger := c.logger.With(zap.String("operation", "process_account_event"))
	startTime := time.Now()

	// Разбор сообщения
	var event originationkafkav1.AccountEvent
	if err := json.Unmarshal(message, &event); err != nil {
		logger.Error("Failed to unmarshal account event",
			zap.Error(err),
			zap.Binary("raw_message", message))
		return fmt.Errorf("failed to unmarshal account event: %w", err)
	}

	// Обогащение логгера контекстной информацией
	logger = logger.With(
		zap.String("ext_account_id", event.ExtAccountId),
		zap.String("currency", event.Currency),
	)

	// Валидация события
	if err := c.validateAccountEvent(&event); err != nil {
		logger.Error("Invalid account event", zap.Error(err))
		return fmt.Errorf("%w: %v", ErrInvalidAccountEvent, err)
	}
	logger = logger.With(zap.String("user_id", event.UserId))

	logger.Info("Received valid account event")

	// Обработка события
	if err := c.processAccountEvent(ctx, &event, logger); err != nil {
		logger.Error("Failed to process account event", zap.Error(err))
		return err
	}

	logger.Info("Successfully processed account event",
		zap.Duration("processing_time", time.Since(startTime)))

	return nil
}

// validateAccountEvent проверяет валидность события аккаунта
func (c *AccountConsumer) validateAccountEvent(event *originationkafkav1.AccountEvent) error {
	if event.ExtAccountId == "" {
		return errors.New("empty ext_account_id")
	}

	if event.UserId == "" {
		return errors.New("empty user_id")
	}

	if event.Currency == "" {
		return errors.New("empty currency")
	}

	return nil
}

// processAccountEvent обрабатывает событие аккаунта
func (c *AccountConsumer) processAccountEvent(
	ctx context.Context,
	event *originationkafkav1.AccountEvent,
	logger *zap.Logger,
) error {
	// Проверяем, существует ли уже аккаунт с таким ext_id
	existingAccount, err := c.accountStore.GetByExtID(ctx, event.ExtAccountId)
	if err == nil {
		// Аккаунт уже существует
		logger.Info("Account already exists, skipping creation",
			zap.String("account_id", existingAccount.ID.String()),
			zap.String("ext_id", existingAccount.ExtID))
		return nil
	}

	if !errors.Is(err, entity.ErrNotFound) {
		// Произошла ошибка при проверке
		logger.Error("Failed to check existing account", zap.Error(err))
		return fmt.Errorf("failed to check existing account: %w", err)
	}

	// Создаем новый аккаунт
	account, err := c.createAccount(ctx, event)
	if err != nil {
		logger.Error("Failed to create account", zap.Error(err))
		return fmt.Errorf("failed to create account: %w", err)
	}

	logger.Info("Successfully created account",
		zap.String("id", account.ID.String()),
		zap.String("ext_id", account.ExtID))

	return nil
}

// createAccount создает новый аккаунт на основе события
func (c *AccountConsumer) createAccount(
	ctx context.Context,
	event *originationkafkav1.AccountEvent,
) (*entity.Account, error) {
	account := c.convertEventToAccount(event)
	// Сохраняем аккаунт в хранилище
	if err := c.accountStore.Create(ctx, account); err != nil {
		return nil, err
	}

	return account, nil
}

func (c *AccountConsumer) convertEventToAccount(event *originationkafkav1.AccountEvent) *entity.Account {
	now := time.Now()
	return &entity.Account{
		ID:        uuid.New(),
		UserID:    event.UserId,
		ExtID:     event.ExtAccountId,
		Balance:   decimal.NewFromInt(0),
		Currency:  event.Currency,
		Status:    entity.AccountStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
