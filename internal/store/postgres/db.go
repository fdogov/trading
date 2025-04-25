package postgres

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/fdogov/trading/internal/config"
)

// DB represents a connection to PostgreSQL database
type DB struct {
	db *sqlx.DB
}

// NewDB creates a new connection to PostgreSQL database
func NewDB(cfg config.Database) (*DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &DB{db: db}, nil
}

// Primary returns the primary database connection (for writing)
func (d *DB) Primary(ctx context.Context) *sqlx.DB {
	return d.db
}

// Replica returns a database replica connection (for reading)
func (d *DB) Replica() *sqlx.DB {
	return d.db
}

// Close closes the database connection
func (d *DB) Close() error {
	return d.db.Close()
}

// Transactor implements the store.DBTransactor interface for working with transactions
type Transactor struct {
	db *DB
}

// NewTransactor creates a new instance of Transactor
func NewTransactor(db *DB) *Transactor {
	return &Transactor{db: db}
}

// Exec executes a function within a transaction
func (t *Transactor) Exec(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := t.db.Primary(ctx).BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	txCtx := context.WithValue(ctx, "tx", tx)

	if err := fn(txCtx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("transaction failed: %v, rollback failed: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
