package postgres

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/fdogov/trading/internal/config"
)

// DB представляет соединение с базой данных PostgreSQL
type DB struct {
	db *sqlx.DB
}

// NewDB создает новое подключение к базе данных PostgreSQL
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

// Primary возвращает соединение с основной базой данных (для записи)
func (d *DB) Primary(ctx context.Context) *sqlx.DB {
	return d.db
}

// Replica возвращает соединение с репликой базы данных (для чтения)
func (d *DB) Replica() *sqlx.DB {
	return d.db
}

// Close закрывает соединение с базой данных
func (d *DB) Close() error {
	return d.db.Close()
}

// Transactor реализует интерфейс store.DBTransactor для работы с транзакциями
type Transactor struct {
	db *DB
}

// NewTransactor создает новый экземпляр Transactor
func NewTransactor(db *DB) *Transactor {
	return &Transactor{db: db}
}

// Exec выполняет функцию в транзакции
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
