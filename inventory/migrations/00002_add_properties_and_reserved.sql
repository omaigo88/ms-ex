-- +goose Up
ALTER TABLE parts ADD COLUMN properties JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE parts ADD COLUMN reserved BIGINT NOT NULL DEFAULT 0;

-- Value Objects per part (must match inventory/pkg/domain's Pointer Union
-- shape and inventory/pkg/repository/memory.go's seed data).
UPDATE parts SET properties = '{"hull": {"mass_kg": 1000, "armor_rating": 20}}'::jsonb
    WHERE uuid = '550e8400-e29b-41d4-a716-446655440001'; -- Aluminum Hull
UPDATE parts SET properties = '{"hull": {"mass_kg": 3000, "armor_rating": 60}}'::jsonb
    WHERE uuid = '550e8400-e29b-41d4-a716-446655440002'; -- Titanium Hull
UPDATE parts SET properties = '{"engine": {"thrust_newton": 5000, "max_hull_mass_kg": 1500}}'::jsonb
    WHERE uuid = '550e8400-e29b-41d4-a716-446655440003'; -- Ion Engine C
UPDATE parts SET properties = '{"engine": {"thrust_newton": 12000, "max_hull_mass_kg": 4000}}'::jsonb
    WHERE uuid = '550e8400-e29b-41d4-a716-446655440004'; -- Ion Engine B
UPDATE parts SET properties = '{"shield": {"capacity_mj": 500, "recharge_rate_mj_per_sec": 10}}'::jsonb
    WHERE uuid = '550e8400-e29b-41d4-a716-446655440005'; -- Energy Shield
UPDATE parts SET properties = '{"weapon": {"damage_rating": 80, "fire_rate_hz": 5}}'::jsonb
    WHERE uuid = '550e8400-e29b-41d4-a716-446655440006'; -- Laser Cannon
UPDATE parts SET properties = '{"hull": {"mass_kg": 5000, "armor_rating": 90}}'::jsonb
    WHERE uuid = '550e8400-e29b-41d4-a716-446655440007'; -- Plasma Hull

-- Bump seed stock well above anything the self-contained test suite could
-- ever consume via real reservations (except Plasma Hull, which stays 0 —
-- it's the dedicated out-of-stock fixture). Keep in sync with
-- inventory/pkg/repository/memory.go.
UPDATE parts SET stock_quantity = 100000 WHERE uuid != '550e8400-e29b-41d4-a716-446655440007';

-- +goose Down
UPDATE parts SET stock_quantity = 10 WHERE uuid = '550e8400-e29b-41d4-a716-446655440001';
UPDATE parts SET stock_quantity = 5 WHERE uuid = '550e8400-e29b-41d4-a716-446655440002';
UPDATE parts SET stock_quantity = 8 WHERE uuid = '550e8400-e29b-41d4-a716-446655440003';
UPDATE parts SET stock_quantity = 3 WHERE uuid = '550e8400-e29b-41d4-a716-446655440004';
UPDATE parts SET stock_quantity = 6 WHERE uuid = '550e8400-e29b-41d4-a716-446655440005';
UPDATE parts SET stock_quantity = 7 WHERE uuid = '550e8400-e29b-41d4-a716-446655440006';

ALTER TABLE parts DROP COLUMN reserved;
ALTER TABLE parts DROP COLUMN properties;
