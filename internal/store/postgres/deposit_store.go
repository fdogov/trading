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

// DepositStore implements the store.DepositStore interface for PostgreSQL
type DepositStore struct {
	db *DB
}

// NewDepositStore creates a new instance of DepositStore
func NewDepositStore(db *DB) *DepositStore {
	return &DepositStore{db: db}
}

// Create creates a new deposit
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

// GetByID gets a deposit by ID
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

// GetByExtID gets a deposit by external ID
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

// GetByIdempotencyKey gets a deposit by idempotency key
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

// Update updates a deposit
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

// UpdateStatus updates the deposit status
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

// UpdateExternalData updates the external ID and status of a deposit
func (s *DepositStore) UpdateExternalData(ctx context.Context, id uuid.UUID, extID string, status entity.DepositStatus) error {
	const query = `
		UPDATE deposits SET
			ext_id = $1,
			status = $2,
			updated_at = NOW()
		WHERE id = $3;
	`

	_, err := s.db.Primary(ctx).ExecContext(ctx, query, extID, status, id)
	if err != nil {
		return fmt.Errorf("failed to update deposit external data: %w", err)
	}

	return nil
}

// GetByAccountID returns all deposits for a specific account
func (s *DepositStore) GetByAccountID(ctx context.Context, accountID uuid.UUID) ([]*entity.Deposit, error) {
	const query = `SELECT * FROM deposits WHERE account_id = $1 ORDER BY created_at DESC;`

	var deposits []*entity.Deposit
	err := sqlx.SelectContext(ctx, s.db.Replica(), &deposits, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deposits by account ID: %w", err)
	}

	if len(deposits) == 0 {
		return []*entity.Deposit{}, nil
	}

	return deposits, nil
}
