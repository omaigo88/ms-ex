-- +goose Up
-- Backfill: orders created before user_uuid existed get the nil UUID as a
-- "legacy / unknown user" placeholder, so the column can become NOT NULL.
UPDATE orders SET user_uuid = '00000000-0000-0000-0000-000000000000' WHERE user_uuid IS NULL;

-- +goose Down
-- Nothing to undo: the placeholder is indistinguishable from data once the
-- column exists, and 00002's Down drops the column anyway.
SELECT 1;
