-- +goose Up
CREATE TABLE IF NOT EXISTS scan_cursor (
    name TEXT PRIMARY KEY,
    last_safe_block_processed BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS scan_cursor;
