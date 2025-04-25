package store

import (
	"context"
	"github.com/shopspring/decimal"

	"github.com/fdogov/trading/internal/entity"
	"github.com/google/uuid"
)

// AccountStore определяет интерфейс для работы с хранилищем аккаунтов
type AccountStore interface {
	// Create создает новый аккаунт
	Create(ctx context.Context, account *entity.Account) error

	// GetByID получает аккаунт по ID
	GetByID(ctx context.Context, id uuid.UUID) (*entity.Account, error)

	// GetByExtID получает аккаунт по внешнему ID
	GetByExtID(ctx context.Context, extID string) (*entity.Account, error)

	// GetByUserID получает аккаунты пользователя по UserID
	GetByUserID(ctx context.Context, userID string) ([]*entity.Account, error)

	// UpdateBalance обновляет баланс аккаунта
	UpdateBalance(ctx context.Context, id uuid.UUID, amount decimal.Decimal) error
}

// OrderStore определяет интерфейс для работы с хранилищем заказов
type OrderStore interface {
	// Create создает новый заказ
	Create(ctx context.Context, order *entity.Order) error

	// GetByID получает заказ по ID
	GetByID(ctx context.Context, id uuid.UUID) (*entity.Order, error)

	// GetByExtID получает заказ по внешнему ID
	GetByExtID(ctx context.Context, extID string) (*entity.Order, error)

	// Update обновляет заказ
	Update(ctx context.Context, order *entity.Order) error

	// UpdateStatus обновляет статус заказа
	UpdateStatus(ctx context.Context, id uuid.UUID, status entity.OrderStatus) error
}

// DepositStore определяет интерфейс для работы с хранилищем депозитов
type DepositStore interface {
	// Create создает новый депозит
	Create(ctx context.Context, deposit *entity.Deposit) error

	// GetByID получает депозит по ID
	GetByID(ctx context.Context, id uuid.UUID) (*entity.Deposit, error)

	// GetByExtID получает депозит по внешнему ID
	GetByExtID(ctx context.Context, extID string) (*entity.Deposit, error)

	// GetByIdempotencyKey получает депозит по ключу идемпотентности
	GetByIdempotencyKey(ctx context.Context, key string) (*entity.Deposit, error)

	// Update обновляет депозит
	Update(ctx context.Context, deposit *entity.Deposit) error

	// UpdateStatus обновляет статус депозита
	UpdateStatus(ctx context.Context, id uuid.UUID, status entity.DepositStatus) error

	// UpdateExternalData обновляет внешний ID и статус депозита
	UpdateExternalData(ctx context.Context, id uuid.UUID, extID string, status entity.DepositStatus) error
}

// EventStore определяет интерфейс для работы с хранилищем событий
type EventStore interface {
	// Create создает новое событие
	Create(ctx context.Context, event *entity.Event) error

	// GetByEventID получает событие по внешнему идентификатору
	GetByEventID(ctx context.Context, id string, eventType entity.EventType) (*entity.Event, error)
}

// DBTransactor определяет интерфейс для работы с транзакциями
type DBTransactor interface {
	// Exec выполняет функцию в транзакции
	Exec(ctx context.Context, fn func(ctx context.Context) error) error
}
