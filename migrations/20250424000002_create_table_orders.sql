-- +migrate Up
CREATE TABLE IF NOT EXISTS orders (
    id              UUID PRIMARY KEY,
    user_id         VARCHAR(255) NOT NULL,
    account_id      UUID NOT NULL REFERENCES accounts (id),
    instrument_id   VARCHAR(255) NOT NULL,
    amount          DECIMAL(18, 6) NOT NULL DEFAULT 0,
    quantity        DECIMAL(18, 6) NOT NULL DEFAULT 0,
    order_type      VARCHAR(50) NOT NULL CHECK (order_type IN ('MARKET', 'LIMIT')),
    side            VARCHAR(50) NOT NULL CHECK (side IN ('BUY', 'SELL')),
    status          VARCHAR(50) NOT NULL CHECK (status IN ('NEW', 'PROCESSING', 'COMPLETED', 'CANCELLED', 'FAILED')),
    description     TEXT,
    ext_id          VARCHAR(255),
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS orders_account_id_idx ON orders (account_id);
CREATE INDEX IF NOT EXISTS orders_user_id_idx ON orders (user_id);
CREATE INDEX IF NOT EXISTS orders_created_at_idx ON orders (created_at);
CREATE UNIQUE INDEX IF NOT EXISTS orders_ext_id_unique_idx ON orders (ext_id) WHERE ext_id IS NOT NULL;

-- +migrate Down
DROP TABLE IF EXISTS orders;
