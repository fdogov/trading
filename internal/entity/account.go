package entity

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Account represents a user's trading account
type Account struct {
	ID        uuid.UUID       `db:"id"`
	UserID    string          `db:"user_id"`
	Balance   decimal.Decimal `db:"balance"`
	Currency  string          `db:"currency"`
	Status    AccountStatus   `db:"status"`
	ExtID     string          `db:"ext_id"`
	CreatedAt time.Time       `db:"created_at"`
	UpdatedAt time.Time       `db:"updated_at"`
}

type AccountStatus string

const (
	AccountStatusUnspecified AccountStatus = "UNSPECIFIED"
	AccountStatusActive      AccountStatus = "ACTIVE"
	AccountStatusInactive    AccountStatus = "INACTIVE"
	AccountStatusBlocked     AccountStatus = "BLOCKED"
)

func (a *Account) IsActive() bool {
	return a.Status == AccountStatusActive
}

func (a *Account) IsBlocked() bool {
	return a.Status == AccountStatusBlocked
}

func (a *Account) IsInactive() bool {
	return a.Status == AccountStatusInactive
}

func (a *Account) HasSufficientFunds(amount decimal.Decimal) bool {
	return a.Balance.GreaterThanOrEqual(amount)
}
