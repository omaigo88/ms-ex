-- +goose Up
CREATE TABLE orders (
    order_uuid       UUID PRIMARY KEY,
    hull_uuid        UUID NOT NULL,
    engine_uuid      UUID NOT NULL,
    shield_uuid      UUID,
    weapon_uuid      UUID,
    total_price      BIGINT NOT NULL,
    transaction_uuid UUID,
    payment_method   TEXT,
    status           TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE orders;
