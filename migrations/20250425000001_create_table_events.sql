-- +migrate Up
CREATE TABLE events (
    id VARCHAR(255) PRIMARY KEY,
    type VARCHAR(50) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

CREATE INDEX idx_events_id_type ON events (id, type);

-- +migrate Down
DROP TABLE events;
