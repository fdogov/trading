package fixtures

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/fdogov/trading/internal/entity"
)

// DepositFixture represents a builder for creating deposit fixtures
type DepositFixture struct {
	deposit *entity.Deposit
}

// NewDeposit creates a new deposit object for tests
func NewDeposit() *DepositFixture {
	now := time.Now()
	return &DepositFixture{
		deposit: &entity.Deposit{
			ID:             uuid.New(),
			AccountID:      uuid.New(),
			Amount:         decimal.NewFromInt(100),
			Currency:       "USD",
			Status:         entity.DepositStatusPending,
			ExtID:          nil, // ExtID изначально nil, будет присвоен позже
			IdempotencyKey: uuid.NewString(),
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
}

// WithID sets the deposit ID
func (f *DepositFixture) WithID(id uuid.UUID) *DepositFixture {
	f.deposit.ID = id
	return f
}

// WithAccountID sets the account ID
func (f *DepositFixture) WithAccountID(accountID uuid.UUID) *DepositFixture {
	f.deposit.AccountID = accountID
	return f
}

// WithAmount sets the deposit amount
func (f *DepositFixture) WithAmount(amount decimal.Decimal) *DepositFixture {
	f.deposit.Amount = amount
	return f
}

// WithCurrency sets the deposit currency
func (f *DepositFixture) WithCurrency(currency string) *DepositFixture {
	f.deposit.Currency = currency
	return f
}

// WithStatus sets the deposit status
func (f *DepositFixture) WithStatus(status entity.DepositStatus) *DepositFixture {
	f.deposit.Status = status
	return f
}

// WithExtID sets the external ID of the deposit
func (f *DepositFixture) WithExtID(extID string) *DepositFixture {
	// Создаем копию строки для корректного присвоения указателю
	extIDCopy := extID
	f.deposit.ExtID = &extIDCopy
	return f
}

// WithIdempotencyKey sets the idempotency key
func (f *DepositFixture) WithIdempotencyKey(key string) *DepositFixture {
	f.deposit.IdempotencyKey = key
	return f
}

// WithCreatedAt sets the deposit creation time
func (f *DepositFixture) WithCreatedAt(createdAt time.Time) *DepositFixture {
	f.deposit.CreatedAt = createdAt
	return f
}

// WithUpdatedAt sets the deposit update time
func (f *DepositFixture) WithUpdatedAt(updatedAt time.Time) *DepositFixture {
	f.deposit.UpdatedAt = updatedAt
	return f
}

// Build returns the prepared deposit object
func (f *DepositFixture) Build() *entity.Deposit {
	return f.deposit
}

// Predefined deposits for tests
var PredefinedDeposits = struct {
	Pending   func(accountID uuid.UUID) *entity.Deposit
	Completed func(accountID uuid.UUID) *entity.Deposit
	Failed    func(accountID uuid.UUID) *entity.Deposit

	PendingWithAmount   func(accountID uuid.UUID, amount decimal.Decimal) *entity.Deposit
	CompletedWithAmount func(accountID uuid.UUID, amount decimal.Decimal) *entity.Deposit

	WithExtID          func(accountID uuid.UUID, extID string) *entity.Deposit
	WithIdempotencyKey func(accountID uuid.UUID, key string) *entity.Deposit
}{
	// Pending deposit
	Pending: func(accountID uuid.UUID) *entity.Deposit {
		return NewDeposit().
			WithAccountID(accountID).
			WithStatus(entity.DepositStatusPending).
			Build()
	},

	// Completed deposit
	Completed: func(accountID uuid.UUID) *entity.Deposit {
		return NewDeposit().
			WithAccountID(accountID).
			WithStatus(entity.DepositStatusCompleted).
			Build()
	},

	// Failed deposit
	Failed: func(accountID uuid.UUID) *entity.Deposit {
		return NewDeposit().
			WithAccountID(accountID).
			WithStatus(entity.DepositStatusFailed).
			Build()
	},

	// Pending deposit with specified amount
	PendingWithAmount: func(accountID uuid.UUID, amount decimal.Decimal) *entity.Deposit {
		return NewDeposit().
			WithAccountID(accountID).
			WithStatus(entity.DepositStatusPending).
			WithAmount(amount).
			Build()
	},

	// Completed deposit with specified amount
	CompletedWithAmount: func(accountID uuid.UUID, amount decimal.Decimal) *entity.Deposit {
		return NewDeposit().
			WithAccountID(accountID).
			WithStatus(entity.DepositStatusCompleted).
			WithAmount(amount).
			Build()
	},

	// Deposit with specified external ID
	WithExtID: func(accountID uuid.UUID, extID string) *entity.Deposit {
		return NewDeposit().
			WithAccountID(accountID).
			WithExtID(extID).
			Build()
	},

	// Deposit with specified idempotency key
	WithIdempotencyKey: func(accountID uuid.UUID, key string) *entity.Deposit {
		return NewDeposit().
			WithAccountID(accountID).
			WithIdempotencyKey(key).
			Build()
	},
}
