package store

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/fdogov/trading/internal/entity"
	"github.com/google/uuid"
)

// AccountStore defines the interface for working with account storage
type AccountStore interface {
	// Create creates a new account
	Create(ctx context.Context, account *entity.Account) error

	// GetByID gets an account by ID
	GetByID(ctx context.Context, id uuid.UUID) (*entity.Account, error)

	// GetByExtID gets an account by external ID
	GetByExtID(ctx context.Context, extID string) (*entity.Account, error)

	// GetByUserID gets user accounts by UserID
	GetByUserID(ctx context.Context, userID string) ([]*entity.Account, error)

	// UpdateBalance updates the account balance
	UpdateBalance(ctx context.Context, id uuid.UUID, amount decimal.Decimal) error
}

// OrderStore defines the interface for working with order storage
type OrderStore interface {
	// Create creates a new order
	Create(ctx context.Context, order *entity.Order) error

	// GetByID gets an order by ID
	GetByID(ctx context.Context, id uuid.UUID) (*entity.Order, error)

	// GetByExtID gets an order by external ID
	GetByExtID(ctx context.Context, extID string) (*entity.Order, error)
	
	// GetByIdempotencyKey gets an order by idempotency key
	GetByIdempotencyKey(ctx context.Context, key string) (*entity.Order, error)

	// Update updates an order
	Update(ctx context.Context, order *entity.Order) error

	// UpdateStatus updates the order status
	UpdateStatus(ctx context.Context, id uuid.UUID, status entity.OrderStatus) error
}

// DepositStore defines the interface for working with deposit storage
type DepositStore interface {
	// Create creates a new deposit
	Create(ctx context.Context, deposit *entity.Deposit) error

	// GetByID gets a deposit by ID
	GetByID(ctx context.Context, id uuid.UUID) (*entity.Deposit, error)

	// GetByExtID gets a deposit by external ID
	GetByExtID(ctx context.Context, extID string) (*entity.Deposit, error)

	// GetByIdempotencyKey gets a deposit by idempotency key
	GetByIdempotencyKey(ctx context.Context, key string) (*entity.Deposit, error)

	// Update updates a deposit
	Update(ctx context.Context, deposit *entity.Deposit) error

	// UpdateStatus updates the deposit status
	UpdateStatus(ctx context.Context, id uuid.UUID, status entity.DepositStatus) error

	// UpdateExternalData updates the external ID and status of a deposit
	UpdateExternalData(ctx context.Context, id uuid.UUID, extID string, status entity.DepositStatus) error
	
	// GetByAccountID returns all deposits for a specific account
	GetByAccountID(ctx context.Context, accountID uuid.UUID) ([]*entity.Deposit, error)
}

// EventStore defines the interface for working with event storage
type EventStore interface {
	// Create creates a new event
	Create(ctx context.Context, event *entity.Event) error

	// GetByEventID gets an event by external identifier
	GetByEventID(ctx context.Context, id string, eventType entity.EventType) (*entity.Event, error)
}

// DBTransactor defines the interface for working with transactions
type DBTransactor interface {
	// Exec executes a function within a transaction
	Exec(ctx context.Context, fn func(ctx context.Context) error) error
}
