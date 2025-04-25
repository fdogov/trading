package entity

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Order представляет торговый заказ
type Order struct {
	ID           uuid.UUID       `db:"id"`
	UserID       string          `db:"user_id"`
	AccountID    uuid.UUID       `db:"account_id"`
	InstrumentID string          `db:"instrument_id"`
	Amount       decimal.Decimal `db:"amount"`
	Quantity     decimal.Decimal `db:"quantity"`
	OrderType    OrderType       `db:"order_type"`
	Side         OrderSide       `db:"side"`
	Status       OrderStatus     `db:"status"`
	Description  string          `db:"description"`
	ExtID        string          `db:"ext_id"`
	CreatedAt    time.Time       `db:"created_at"`
	UpdatedAt    time.Time       `db:"updated_at"`
}

type OrderType string

const (
	OrderTypeUnspecified OrderType = "UNSPECIFIED"
	OrderTypeMarket      OrderType = "MARKET"
	OrderTypeLimit       OrderType = "LIMIT"
)

type OrderSide string

const (
	OrderSideUnspecified OrderSide = "UNSPECIFIED"
	OrderSideBuy         OrderSide = "BUY"
	OrderSideSell        OrderSide = "SELL"
)

type OrderStatus string

const (
	OrderStatusUnspecified OrderStatus = "UNSPECIFIED"
	OrderStatusNew         OrderStatus = "NEW"
	OrderStatusProcessing  OrderStatus = "PROCESSING"
	OrderStatusCompleted   OrderStatus = "COMPLETED"
	OrderStatusCancelled   OrderStatus = "CANCELLED"
	OrderStatusFailed      OrderStatus = "FAILED"
)

func (o *Order) IsCompleted() bool {
	return o.Status == OrderStatusCompleted
}

func (o *Order) IsFailed() bool {
	return o.Status == OrderStatusFailed
}

func (o *Order) IsCancelled() bool {
	return o.Status == OrderStatusCancelled
}

func (o *Order) IsTerminal() bool {
	return o.IsCompleted() || o.IsFailed() || o.IsCancelled()
}

func (o *Order) IsBuy() bool {
	return o.Side == OrderSideBuy
}

func (o *Order) IsSell() bool {
	return o.Side == OrderSideSell
}
