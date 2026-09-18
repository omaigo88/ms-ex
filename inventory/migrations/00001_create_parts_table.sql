-- +goose Up
CREATE TABLE parts (
    uuid           UUID PRIMARY KEY,
    name           TEXT NOT NULL,
    description    TEXT NOT NULL,
    price          BIGINT NOT NULL,
    part_type      SMALLINT NOT NULL,
    stock_quantity BIGINT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed data. part_type: 1=HULL, 2=ENGINE, 3=SHIELD, 4=WEAPON
-- (must match inventoryv1.PartType in shared/proto/inventory/v1/inventory.proto).
INSERT INTO parts (uuid, name, description, price, part_type, stock_quantity) VALUES
    ('550e8400-e29b-41d4-a716-446655440001', 'Aluminum Hull', 'Lightweight hull for small ships', 500000, 1, 10),
    ('550e8400-e29b-41d4-a716-446655440002', 'Titanium Hull', 'Durable hull for medium ships', 1500000, 1, 5),
    ('550e8400-e29b-41d4-a716-446655440003', 'Ion Engine C', 'Basic class C ion engine', 300000, 2, 8),
    ('550e8400-e29b-41d4-a716-446655440004', 'Ion Engine B', 'Improved class B ion engine', 800000, 2, 3),
    ('550e8400-e29b-41d4-a716-446655440005', 'Energy Shield', 'Standard energy shield', 400000, 3, 6),
    ('550e8400-e29b-41d4-a716-446655440006', 'Laser Cannon', 'Precision laser cannon', 250000, 4, 7),
    ('550e8400-e29b-41d4-a716-446655440007', 'Plasma Hull', 'Experimental hull (out of stock)', 2000000, 1, 0);

-- +goose Down
DROP TABLE parts;
