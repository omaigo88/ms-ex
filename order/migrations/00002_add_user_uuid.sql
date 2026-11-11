-- +goose Up
-- Expand: add the column as nullable so existing rows stay valid.
ALTER TABLE orders ADD COLUMN user_uuid UUID;

-- +goose Down
ALTER TABLE orders DROP COLUMN user_uuid;
