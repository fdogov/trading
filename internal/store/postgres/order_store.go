package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/fdogov/trading/internal/entity"
)

// OrderStore реализует интерфейс store.OrderStore для PostgreSQL
type OrderStore struct {
	db *DB
}

// NewOrderStore создает новый экземпляр OrderStore
func NewOrderStore(db *DB) *OrderStore {
	return &OrderStore{db: db}
}

// Create создает новый заказ
func (s *OrderStore) Create(ctx context.Context, order *entity.Order) error {
	const query = `
		INSERT INTO orders (
			id, user_id, account_id, instrument_id, amount, quantity, 
			order_type, side, status, description, ext_id, created_at, updated_at
		) VALUES (
			:id, :user_id, :account_id, :instrument_id, :amount, :quantity, 
			:order_type, :side, :status, :description, :ext_id, :created_at, :updated_at
		);
	`

	_, err := sqlx.NamedExecContext(ctx, s.db.Primary(ctx), query, order)
	if err != nil {
		return fmt.Errorf("failed to create order: %w", err)
	}

	return nil
}

// GetByID получает заказ по ID
func (s *OrderStore) GetByID(ctx context.Context, id uuid.UUID) (*entity.Order, error) {
	const query = `SELECT * FROM orders WHERE id = $1;`

	var order entity.Order
	err := sqlx.GetContext(ctx, s.db.Replica(), &order, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, entity.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get order by ID: %w", err)
	}

	return &order, nil
}

// GetByExtID получает заказ по внешнему ID
func (s *OrderStore) GetByExtID(ctx context.Context, extID string) (*entity.Order, error) {
	const query = `SELECT * FROM orders WHERE ext_id = $1;`

	var order entity.Order
	err := sqlx.GetContext(ctx, s.db.Replica(), &order, query, extID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, entity.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get order by ext ID: %w", err)
	}

	return &order, nil
}

// Update обновляет заказ
func (s *OrderStore) Update(ctx context.Context, order *entity.Order) error {
	const query = `
		UPDATE orders SET
			user_id = :user_id,
			account_id = :account_id,
			instrument_id = :instrument_id,
			amount = :amount,
			quantity = :quantity,
			order_type = :order_type,
			side = :side,
			status = :status,
			description = :description,
			ext_id = :ext_id,
			updated_at = :updated_at
		WHERE id = :id;
	`

	_, err := sqlx.NamedExecContext(ctx, s.db.Primary(ctx), query, order)
	if err != nil {
		return fmt.Errorf("failed to update order: %w", err)
	}

	return nil
}

// UpdateStatus обновляет статус заказа
func (s *OrderStore) UpdateStatus(ctx context.Context, id uuid.UUID, status entity.OrderStatus) error {
	const query = `
		UPDATE orders SET
			status = $1,
			updated_at = NOW()
		WHERE id = $2;
	`

	_, err := s.db.Primary(ctx).ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	return nil
}
