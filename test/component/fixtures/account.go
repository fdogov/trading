package fixtures

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/fdogov/trading/internal/entity"
)

// AccountFixture представляет строитель для создания фикстур аккаунтов
type AccountFixture struct {
	account *entity.Account
}

// NewAccount создает новый объект аккаунта для тестов
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

// WithID устанавливает ID аккаунта
func (f *AccountFixture) WithID(id uuid.UUID) *AccountFixture {
	f.account.ID = id
	return f
}

// WithUserID устанавливает ID пользователя
func (f *AccountFixture) WithUserID(userID string) *AccountFixture {
	f.account.UserID = userID
	return f
}

// WithExtID устанавливает внешний ID аккаунта
func (f *AccountFixture) WithExtID(extID string) *AccountFixture {
	f.account.ExtID = extID
	return f
}

// WithBalance устанавливает баланс аккаунта
func (f *AccountFixture) WithBalance(balance decimal.Decimal) *AccountFixture {
	f.account.Balance = balance
	return f
}

// WithCurrency устанавливает валюту аккаунта
func (f *AccountFixture) WithCurrency(currency string) *AccountFixture {
	f.account.Currency = currency
	return f
}

// WithStatus устанавливает статус аккаунта
func (f *AccountFixture) WithStatus(status entity.AccountStatus) *AccountFixture {
	f.account.Status = status
	return f
}

// WithCreatedAt устанавливает время создания аккаунта
func (f *AccountFixture) WithCreatedAt(createdAt time.Time) *AccountFixture {
	f.account.CreatedAt = createdAt
	return f
}

// WithUpdatedAt устанавливает время обновления аккаунта
func (f *AccountFixture) WithUpdatedAt(updatedAt time.Time) *AccountFixture {
	f.account.UpdatedAt = updatedAt
	return f
}

// Build возвращает готовый объект аккаунта
func (f *AccountFixture) Build() *entity.Account {
	return f.account
}

// Предопределенные аккаунты для тестов
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
	// Активный аккаунт
	Active: func(userID string) *entity.Account {
		return NewAccount().
			WithUserID(userID).
			WithStatus(entity.AccountStatusActive).
			Build()
	},

	// Неактивный аккаунт
	Inactive: func(userID string) *entity.Account {
		return NewAccount().
			WithUserID(userID).
			WithStatus(entity.AccountStatusInactive).
			Build()
	},

	// Заблокированный аккаунт
	Blocked: func(userID string) *entity.Account {
		return NewAccount().
			WithUserID(userID).
			WithStatus(entity.AccountStatusBlocked).
			Build()
	},

	// Аккаунт с указанным балансом в USD
	WithBalanceUSD: func(userID string, balance decimal.Decimal) *entity.Account {
		return NewAccount().
			WithUserID(userID).
			WithStatus(entity.AccountStatusActive).
			WithBalance(balance).
			WithCurrency("USD").
			Build()
	},

	// Аккаунт с указанным балансом в EUR
	WithBalanceEUR: func(userID string, balance decimal.Decimal) *entity.Account {
		return NewAccount().
			WithUserID(userID).
			WithStatus(entity.AccountStatusActive).
			WithBalance(balance).
			WithCurrency("EUR").
			Build()
	},

	// Активный аккаунт с балансом 1000 USD и указанным ID
	ActiveWithCash1000: func(userID string, id uuid.UUID) *entity.Account {
		return NewAccount().
			WithID(id).
			WithUserID(userID).
			WithStatus(entity.AccountStatusActive).
			WithBalance(decimal.NewFromInt(1000)).
			WithCurrency("USD").
			Build()
	},

	// Активный аккаунт с балансом 5000 USD и указанным ID
	ActiveWithCash5000: func(userID string, id uuid.UUID) *entity.Account {
		return NewAccount().
			WithID(id).
			WithUserID(userID).
			WithStatus(entity.AccountStatusActive).
			WithBalance(decimal.NewFromInt(5000)).
			WithCurrency("USD").
			Build()
	},

	// Неактивный аккаунт с балансом 1000 USD и указанным ID
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
