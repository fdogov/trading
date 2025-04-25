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

// DepositStore реализует интерфейс store.DepositStore для PostgreSQL
type DepositStore struct {
	db *DB
}

// NewDepositStore создает новый экземпляр DepositStore
func NewDepositStore(db *DB) *DepositStore {
	return &DepositStore{db: db}
}

// Create создает новый депозит
func (s *DepositStore) Create(ctx context.Context, deposit *entity.Deposit) error {
	const query = `
		INSERT INTO deposits (
			id, account_id, amount, currency, status, ext_id, idempotency_key, created_at, updated_at
		) VALUES (
			:id, :account_id, :amount, :currency, :status, :ext_id, :idempotency_key, :created_at, :updated_at
		);
	`

	_, err := sqlx.NamedExecContext(ctx, s.db.Primary(ctx), query, deposit)
	if err != nil {
		return fmt.Errorf("failed to create deposit: %w", err)
	}

	return nil
}

// GetByID получает депозит по ID
func (s *DepositStore) GetByID(ctx context.Context, id uuid.UUID) (*entity.Deposit, error) {
	const query = `SELECT * FROM deposits WHERE id = $1;`

	var deposit entity.Deposit
	err := sqlx.GetContext(ctx, s.db.Replica(), &deposit, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, entity.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get deposit by ID: %w", err)
	}

	return &deposit, nil
}

// GetByExtID получает депозит по внешнему ID
func (s *DepositStore) GetByExtID(ctx context.Context, extID string) (*entity.Deposit, error) {
	const query = `SELECT * FROM deposits WHERE ext_id = $1;`

	var deposit entity.Deposit
	err := sqlx.GetContext(ctx, s.db.Replica(), &deposit, query, extID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, entity.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get deposit by ext ID: %w", err)
	}

	return &deposit, nil
}

// GetByIdempotencyKey получает депозит по ключу идемпотентности
func (s *DepositStore) GetByIdempotencyKey(ctx context.Context, key string) (*entity.Deposit, error) {
	const query = `SELECT * FROM deposits WHERE idempotency_key = $1;`

	var deposit entity.Deposit
	err := sqlx.GetContext(ctx, s.db.Replica(), &deposit, query, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, entity.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get deposit by idempotency key: %w", err)
	}

	return &deposit, nil
}

// Update обновляет депозит
func (s *DepositStore) Update(ctx context.Context, deposit *entity.Deposit) error {
	const query = `
		UPDATE deposits SET
			account_id = :account_id,
			amount = :amount,
			currency = :currency,
			status = :status,
			ext_id = :ext_id,
			updated_at = :updated_at
		WHERE id = :id;
	`

	_, err := sqlx.NamedExecContext(ctx, s.db.Primary(ctx), query, deposit)
	if err != nil {
		return fmt.Errorf("failed to update deposit: %w", err)
	}

	return nil
}

// UpdateStatus обновляет статус депозита
func (s *DepositStore) UpdateStatus(ctx context.Context, id uuid.UUID, status entity.DepositStatus) error {
	const query = `
		UPDATE deposits SET
			status = $1,
			updated_at = NOW()
		WHERE id = $2;
	`

	_, err := s.db.Primary(ctx).ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update deposit status: %w", err)
	}

	return nil
}
