-- +migrate Up
CREATE TABLE IF NOT EXISTS deposits (
    id              UUID PRIMARY KEY,
    account_id      UUID NOT NULL REFERENCES accounts (id),
    amount          DECIMAL(18, 6) NOT NULL DEFAULT 0,
    currency        VARCHAR(10) NOT NULL,
    status          VARCHAR(50) NOT NULL CHECK (status IN ('PENDING', 'COMPLETED', 'FAILED')),
    ext_id          VARCHAR(255),
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS deposits_account_id_idx ON deposits (account_id);
CREATE INDEX IF NOT EXISTS deposits_created_at_idx ON deposits (created_at);
CREATE UNIQUE INDEX IF NOT EXISTS deposits_ext_id_unique_idx ON deposits (ext_id) WHERE ext_id IS NOT NULL;

-- +migrate Down
DROP TABLE IF EXISTS deposits;
