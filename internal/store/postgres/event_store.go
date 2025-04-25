package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/fdogov/trading/internal/entity"
)

// EventStore implements the store.EventStore interface for PostgreSQL
type EventStore struct {
	db *DB
}

// NewEventStore creates a new instance of EventStore
func NewEventStore(db *DB) *EventStore {
	return &EventStore{
		db: db,
	}
}

// Create creates a new event
func (s *EventStore) Create(ctx context.Context, event *entity.Event) error {
	query := `
		INSERT INTO events (id, type, created_at)
		VALUES ($1, $2, $3)
	`

	_, err := s.db.Primary(ctx).ExecContext(
		ctx,
		query,
		event.ID,
		event.Type,
		event.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create event: %w", err)
	}

	return nil
}

// GetByEventID gets an event by external identifier and type
func (s *EventStore) GetByEventID(ctx context.Context, id string, eventType entity.EventType) (*entity.Event, error) {
	query := `
		SELECT *
		FROM events
		WHERE id = $1 AND type = $2
		LIMIT 1
	`

	var event entity.Event
	err := s.db.Replica().GetContext(ctx, &event, query, id, eventType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, entity.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get event by external ID: %w", err)
	}

	return &event, nil
}
