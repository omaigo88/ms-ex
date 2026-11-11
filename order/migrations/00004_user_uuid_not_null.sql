-- +goose Up
-- Contract: every row has a user now, enforce it going forward.
ALTER TABLE orders ALTER COLUMN user_uuid SET NOT NULL;

-- +goose Down
ALTER TABLE orders ALTER COLUMN user_uuid DROP NOT NULL;
