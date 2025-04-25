package postgres

import (
	"fmt"

	_ "github.com/lib/pq" // Драйвер PostgreSQL

	"github.com/fdogov/trading/internal/config"
	"github.com/fdogov/trading/internal/store"
)

// Store реализует все хранилища данных
type Store struct {
	db           *DB
	accountStore store.AccountStore
	orderStore   store.OrderStore
	depositStore store.DepositStore
	dbTransactor store.DBTransactor
}

// NewStore создает новый экземпляр Store
func NewStore(cfg config.Database) (*Store, error) {
	db, err := NewDB(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create database connection: %w", err)
	}

	accountStore := NewAccountStore(db)
	orderStore := NewOrderStore(db)
	depositStore := NewDepositStore(db)
	dbTransactor := NewTransactor(db)

	return &Store{
		db:           db,
		accountStore: accountStore,
		orderStore:   orderStore,
		depositStore: depositStore,
		dbTransactor: dbTransactor,
	}, nil
}

// AccountStore возвращает реализацию AccountStore
func (s *Store) AccountStore() store.AccountStore {
	return s.accountStore
}

// OrderStore возвращает реализацию OrderStore
func (s *Store) OrderStore() store.OrderStore {
	return s.orderStore
}

// DepositStore возвращает реализацию DepositStore
func (s *Store) DepositStore() store.DepositStore {
	return s.depositStore
}

// DBTransactor возвращает реализацию DBTransactor
func (s *Store) DBTransactor() store.DBTransactor {
	return s.dbTransactor
}

// Close закрывает соединение с базой данных
func (s *Store) Close() error {
	return s.db.Close()
}
