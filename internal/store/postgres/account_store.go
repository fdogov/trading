package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/shopspring/decimal"

	"github.com/fdogov/trading/internal/entity"
)

// AccountStore implements the store.AccountStore interface for PostgreSQL
type AccountStore struct {
	db *DB
}

// NewAccountStore creates a new instance of AccountStore
func NewAccountStore(db *DB) *AccountStore {
	return &AccountStore{db: db}
}

// Create creates a new account
func (s *AccountStore) Create(ctx context.Context, account *entity.Account) error {
	const query = `
		INSERT INTO accounts (
			id, user_id, ext_id, balance, currency, status, created_at, updated_at
		) VALUES (
			:id, :user_id, :ext_id, :balance, :currency, :status, :created_at, :updated_at
		);
	`

	_, err := sqlx.NamedExecContext(ctx, s.db.Primary(ctx), query, account)
	if err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}

	return nil
}

// GetByID gets an account by ID
func (s *AccountStore) GetByID(ctx context.Context, id uuid.UUID) (*entity.Account, error) {
	const query = `SELECT * FROM accounts WHERE id = $1;`

	var account entity.Account
	err := sqlx.GetContext(ctx, s.db.Replica(), &account, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, entity.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get account by ID: %w", err)
	}

	return &account, nil
}

// GetByUserID gets accounts by UserID
func (s *AccountStore) GetByUserID(ctx context.Context, userID string) ([]*entity.Account, error) {
	const query = `SELECT * FROM accounts WHERE user_id = $1;`

	var accounts []*entity.Account
	err := sqlx.SelectContext(ctx, s.db.Replica(), &accounts, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get accounts by user ID: %w", err)
	}

	return accounts, nil
}

// GetByExtID gets an account by external ID
func (s *AccountStore) GetByExtID(ctx context.Context, extID string) (*entity.Account, error) {
	const query = `SELECT * FROM accounts WHERE ext_id = $1;`

	var account entity.Account
	err := sqlx.GetContext(ctx, s.db.Replica(), &account, query, extID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, entity.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get account by ext ID: %w", err)
	}

	return &account, nil
}

// Update updates an account
func (s *AccountStore) Update(ctx context.Context, account *entity.Account) error {
	const query = `
		UPDATE accounts SET
			user_id = :user_id,
			ext_id = :ext_id,
			balance = :balance,
			currency = :currency,
			status = :status,
			updated_at = :updated_at
		WHERE id = :id;
	`

	_, err := sqlx.NamedExecContext(ctx, s.db.Primary(ctx), query, account)
	if err != nil {
		return fmt.Errorf("failed to update account: %w", err)
	}

	return nil
}

// UpdateBalance updates the account balance
func (s *AccountStore) UpdateBalance(ctx context.Context, id uuid.UUID, amount decimal.Decimal) error {
	const query = `
		UPDATE accounts SET
			balance = $1,
			updated_at = NOW()
		WHERE id = $2;
	`

	_, err := s.db.Primary(ctx).ExecContext(ctx, query, amount, id)
	if err != nil {
		return fmt.Errorf("failed to update account balance: %w", err)
	}

	return nil
}

// GetByExtIDAndUserID gets an account by external ID and UserID
func (s *AccountStore) GetByExtIDAndUserID(ctx context.Context, extID, userID string) (*entity.Account, error) {
	const query = `SELECT * FROM accounts WHERE ext_id = $1 AND user_id = $2;`

	var account entity.Account
	err := sqlx.GetContext(ctx, s.db.Replica(), &account, query, extID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, entity.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get account by ext ID and user ID: %w", err)
	}

	return &account, nil
}
