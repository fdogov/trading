package entity

import (
	"time"
)

// EventType represents the type of event
type EventType string

const (
	EventTypeDeposit EventType = "DEPOSIT"
	EventTypeOrder   EventType = "ORDER"
)

// Event represents a processing event in the system for idempotency
type Event struct {
	ID        string    `db:"id"`
	Type      EventType `db:"type"`
	CreatedAt time.Time `db:"created_at"`
}

// NewEvent creates a new Event instance
func NewEvent(eventType EventType, eventID string, createdAt time.Time) *Event {
	return &Event{
		ID:        eventID,
		Type:      eventType,
		CreatedAt: createdAt,
	}
}
