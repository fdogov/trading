package fixtures

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/fdogov/trading/internal/entity"
)

// AccountFixture represents a builder for creating account fixtures
type AccountFixture struct {
	account *entity.Account
}

// NewAccount creates a new account object for tests
func NewAccount() *AccountFixture {
	now := time.Now()
	return &AccountFixture{
		account: &entity.Account{
			ID:        uuid.New(),
			UserID:    uuid.NewString(),
			ExtID:     uuid.NewString(),
			Balance:   decimal.NewFromInt(0),
			Currency:  "USD",
			Status:    entity.AccountStatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}

// WithID sets the account ID
func (f *AccountFixture) WithID(id uuid.UUID) *AccountFixture {
	f.account.ID = id
	return f
}

// WithUserID sets the user ID
func (f *AccountFixture) WithUserID(userID string) *AccountFixture {
	f.account.UserID = userID
	return f
}

// WithExtID sets the external ID of the account
func (f *AccountFixture) WithExtID(extID string) *AccountFixture {
	f.account.ExtID = extID
	return f
}

// WithBalance sets the account balance
func (f *AccountFixture) WithBalance(balance decimal.Decimal) *AccountFixture {
	f.account.Balance = balance
	return f
}

// WithCurrency sets the account currency
func (f *AccountFixture) WithCurrency(currency string) *AccountFixture {
	f.account.Currency = currency
	return f
}

// WithStatus sets the account status
func (f *AccountFixture) WithStatus(status entity.AccountStatus) *AccountFixture {
	f.account.Status = status
	return f
}

// WithCreatedAt sets the account creation time
func (f *AccountFixture) WithCreatedAt(createdAt time.Time) *AccountFixture {
	f.account.CreatedAt = createdAt
	return f
}

// WithUpdatedAt sets the account update time
func (f *AccountFixture) WithUpdatedAt(updatedAt time.Time) *AccountFixture {
	f.account.UpdatedAt = updatedAt
	return f
}

// Build returns the prepared account object
func (f *AccountFixture) Build() *entity.Account {
	return f.account
}

// Predefined accounts for tests
var PredefinedAccounts = struct {
	Active               func(userID string) *entity.Account
	Inactive             func(userID string) *entity.Account
	Blocked              func(userID string) *entity.Account
	WithBalanceUSD       func(userID string, balance decimal.Decimal) *entity.Account
	WithBalanceEUR       func(userID string, balance decimal.Decimal) *entity.Account
	ActiveWithCash1000   func(userID string, id uuid.UUID) *entity.Account
	ActiveWithCash5000   func(userID string, id uuid.UUID) *entity.Account
	InactiveWithCash1000 func(userID string, id uuid.UUID) *entity.Account
}{
	// Active account
	Active: func(userID string) *entity.Account {
		return NewAccount().
			WithUserID(userID).
			WithStatus(entity.AccountStatusActive).
			Build()
	},

	// Inactive account
	Inactive: func(userID string) *entity.Account {
		return NewAccount().
			WithUserID(userID).
			WithStatus(entity.AccountStatusInactive).
			Build()
	},

	// Blocked account
	Blocked: func(userID string) *entity.Account {
		return NewAccount().
			WithUserID(userID).
			WithStatus(entity.AccountStatusBlocked).
			Build()
	},

	// Account with specified balance in USD
	WithBalanceUSD: func(userID string, balance decimal.Decimal) *entity.Account {
		return NewAccount().
			WithUserID(userID).
			WithStatus(entity.AccountStatusActive).
			WithBalance(balance).
			WithCurrency("USD").
			Build()
	},

	// Account with specified balance in EUR
	WithBalanceEUR: func(userID string, balance decimal.Decimal) *entity.Account {
		return NewAccount().
			WithUserID(userID).
			WithStatus(entity.AccountStatusActive).
			WithBalance(balance).
			WithCurrency("EUR").
			Build()
	},

	// Active account with 1000 USD balance and specified ID
	ActiveWithCash1000: func(userID string, id uuid.UUID) *entity.Account {
		return NewAccount().
			WithID(id).
			WithUserID(userID).
			WithStatus(entity.AccountStatusActive).
			WithBalance(decimal.NewFromInt(1000)).
			WithCurrency("USD").
			Build()
	},

	// Active account with 5000 USD balance and specified ID
	ActiveWithCash5000: func(userID string, id uuid.UUID) *entity.Account {
		return NewAccount().
			WithID(id).
			WithUserID(userID).
			WithStatus(entity.AccountStatusActive).
			WithBalance(decimal.NewFromInt(5000)).
			WithCurrency("USD").
			Build()
	},

	// Inactive account with 1000 USD balance and specified ID
	InactiveWithCash1000: func(userID string, id uuid.UUID) *entity.Account {
		return NewAccount().
			WithID(id).
			WithUserID(userID).
			WithStatus(entity.AccountStatusInactive).
			WithBalance(decimal.NewFromInt(1000)).
			WithCurrency("USD").
			Build()
	},
}
