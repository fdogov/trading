-- +migrate Up
CREATE TABLE IF NOT EXISTS accounts (
    id                UUID PRIMARY KEY,
    user_id           VARCHAR(255) NOT NULL,
    ext_id            VARCHAR(255) NOT NULL UNIQUE,
    balance           DECIMAL(18, 6) NOT NULL DEFAULT 0,
    currency          VARCHAR(10) NOT NULL,
    status            VARCHAR(50) NOT NULL CHECK (status IN ('ACTIVE', 'INACTIVE', 'BLOCKED')),
    created_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS accounts_user_id_idx ON accounts (user_id);
CREATE INDEX IF NOT EXISTS accounts_created_at_idx ON accounts (created_at);

-- +migrate Down
DROP TABLE IF EXISTS accounts;
