package postgres

import (
	"fmt"

	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/fdogov/trading/internal/config"
	"github.com/fdogov/trading/internal/store"
)

// Store implements all data stores
type Store struct {
	db           *DB
	accountStore store.AccountStore
	orderStore   store.OrderStore
	depositStore store.DepositStore
	eventStore   store.EventStore
	dbTransactor store.DBTransactor
}

// NewStore creates a new instance of Store
func NewStore(cfg config.Database) (*Store, error) {
	db, err := NewDB(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create database connection: %w", err)
	}

	accountStore := NewAccountStore(db)
	orderStore := NewOrderStore(db)
	depositStore := NewDepositStore(db)
	eventStore := NewEventStore(db)
	dbTransactor := NewTransactor(db)

	return &Store{
		db:           db,
		accountStore: accountStore,
		orderStore:   orderStore,
		depositStore: depositStore,
		eventStore:   eventStore,
		dbTransactor: dbTransactor,
	}, nil
}

// AccountStore returns the AccountStore implementation
func (s *Store) AccountStore() store.AccountStore {
	return s.accountStore
}

// OrderStore returns the OrderStore implementation
func (s *Store) OrderStore() store.OrderStore {
	return s.orderStore
}

// DepositStore returns the DepositStore implementation
func (s *Store) DepositStore() store.DepositStore {
	return s.depositStore
}

// EventStore returns the EventStore implementation
func (s *Store) EventStore() store.EventStore {
	return s.eventStore
}

// DBTransactor returns the DBTransactor implementation
func (s *Store) DBTransactor() store.DBTransactor {
	return s.dbTransactor
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}
