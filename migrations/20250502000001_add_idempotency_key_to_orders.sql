-- +migrate Up
ALTER TABLE orders ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(255);
CREATE UNIQUE INDEX IF NOT EXISTS orders_idempotency_key_unique_idx ON orders (idempotency_key) WHERE idempotency_key IS NOT NULL;

-- +migrate Down
DROP INDEX IF EXISTS orders_idempotency_key_unique_idx;
ALTER TABLE orders DROP COLUMN IF EXISTS idempotency_key;