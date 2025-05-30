package entity

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Deposit represents a deposit of funds to an account
type Deposit struct {
	ID             uuid.UUID       `db:"id"`
	AccountID      uuid.UUID       `db:"account_id"`
	Amount         decimal.Decimal `db:"amount"`
	Currency       string          `db:"currency"`
	Status         DepositStatus   `db:"status"`
	ExtID          *string         `db:"ext_id"`
	IdempotencyKey string          `db:"idempotency_key"`
	CreatedAt      time.Time       `db:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at"`
}

type DepositStatus string

func (d DepositStatus) String() string {
	return string(d)
}

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

func (d *Deposit) IsTerminated() bool {
	return d.IsCompleted() || d.IsFailed()
}
