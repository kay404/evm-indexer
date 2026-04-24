-- +goose Up
ALTER TABLE scan_cursor ADD COLUMN last_block_hash VARCHAR(66) NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE scan_cursor DROP COLUMN last_block_hash;
