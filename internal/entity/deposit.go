package entity

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Deposit представляет депозит средств на аккаунт
type Deposit struct {
	ID        uuid.UUID       `db:"id"`
	AccountID uuid.UUID       `db:"account_id"`
	Amount    decimal.Decimal `db:"amount"`
	Currency  string          `db:"currency"`
	Status    DepositStatus   `db:"status"`
	ExtID     string          `db:"ext_id"`
	CreatedAt time.Time       `db:"created_at"`
	UpdatedAt time.Time       `db:"updated_at"`
}

type DepositStatus string

const (
	DepositStatusUnspecified DepositStatus = "UNSPECIFIED"
	DepositStatusPending     DepositStatus = "PENDING"
	DepositStatusCompleted   DepositStatus = "COMPLETED"
	DepositStatusFailed      DepositStatus = "FAILED"
)

func (d *Deposit) IsPending() bool {
	return d.Status == DepositStatusPending
}

func (d *Deposit) IsCompleted() bool {
	return d.Status == DepositStatusCompleted
}

func (d *Deposit) IsFailed() bool {
	return d.Status == DepositStatusFailed
}
