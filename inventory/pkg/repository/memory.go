package repository

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/omaigo88/inventory/pkg/domain"
)

// memoryRepository is an in-memory Repository implementation, pre-populated with seed data.
type memoryRepository struct {
	mu    sync.RWMutex
	parts map[uuid.UUID]PartRecord
}

// NewMemoryRepository creates a repository pre-populated with seed data.
//
// Properties/stock here must stay in sync with
// inventory/migrations/00002_add_properties_and_reserved.sql — same UUIDs,
// same Value Object shapes. Stock is deliberately generous (not the
// migration's original small numbers): order/tests/api_test.go reuses these
// same parts across ~25 separate CreateOrder calls sharing this one
// repository instance for the whole test run, and CreateOrder now actually
// reserves stock instead of just reading it.
func NewMemoryRepository() Repository {
	now := time.Now()

	return &memoryRepository{
		parts: map[uuid.UUID]PartRecord{
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"): {
				UUID:          uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
				Name:          "Aluminum Hull",
				Description:   "Lightweight hull for small ships",
				Price:         500000, // 5000₽
				PartType:      int32(domain.PartTypeHull),
				Properties:    []byte(`{"hull":{"mass_kg":1000,"armor_rating":20}}`),
				StockQuantity: 100000,
				CreatedAt:     now,
			},
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440002"): {
				UUID:          uuid.MustParse("550e8400-e29b-41d4-a716-446655440002"),
				Name:          "Titanium Hull",
				Description:   "Durable hull for medium ships",
				Price:         1500000, // 15000₽
				PartType:      int32(domain.PartTypeHull),
				Properties:    []byte(`{"hull":{"mass_kg":3000,"armor_rating":60}}`),
				StockQuantity: 100000,
				CreatedAt:     now,
			},
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440003"): {
				UUID:          uuid.MustParse("550e8400-e29b-41d4-a716-446655440003"),
				Name:          "Ion Engine C",
				Description:   "Basic class C ion engine",
				Price:         300000, // 3000₽
				PartType:      int32(domain.PartTypeEngine),
				Properties:    []byte(`{"engine":{"thrust_newton":5000,"max_hull_mass_kg":1500}}`),
				StockQuantity: 100000,
				CreatedAt:     now,
			},
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440004"): {
				UUID:          uuid.MustParse("550e8400-e29b-41d4-a716-446655440004"),
				Name:          "Ion Engine B",
				Description:   "Improved class B ion engine",
				Price:         800000, // 8000₽
				PartType:      int32(domain.PartTypeEngine),
				Properties:    []byte(`{"engine":{"thrust_newton":12000,"max_hull_mass_kg":4000}}`),
				StockQuantity: 100000,
				CreatedAt:     now,
			},
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440005"): {
				UUID:          uuid.MustParse("550e8400-e29b-41d4-a716-446655440005"),
				Name:          "Energy Shield",
				Description:   "Standard energy shield",
				Price:         400000, // 4000₽
				PartType:      int32(domain.PartTypeShield),
				Properties:    []byte(`{"shield":{"capacity_mj":500,"recharge_rate_mj_per_sec":10}}`),
				StockQuantity: 100000,
				CreatedAt:     now,
			},
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440006"): {
				UUID:          uuid.MustParse("550e8400-e29b-41d4-a716-446655440006"),
				Name:          "Laser Cannon",
				Description:   "Precision laser cannon",
				Price:         250000, // 2500₽
				PartType:      int32(domain.PartTypeWeapon),
				Properties:    []byte(`{"weapon":{"damage_rating":80,"fire_rate_hz":5}}`),
				StockQuantity: 100000,
				CreatedAt:     now,
			},
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440007"): {
				UUID:          uuid.MustParse("550e8400-e29b-41d4-a716-446655440007"),
				Name:          "Plasma Hull",
				Description:   "Experimental hull (out of stock)",
				Price:         2000000, // 20000₽
				PartType:      int32(domain.PartTypeHull),
				Properties:    []byte(`{"hull":{"mass_kg":5000,"armor_rating":90}}`),
				StockQuantity: 0,
				CreatedAt:     now,
			},
		},
	}
}

// GetPart returns a part by UUID.
func (r *memoryRepository) GetPart(_ context.Context, id uuid.UUID) (domain.Part, error) {
	r.mu.RLock()
	rec, ok := r.parts[id]
	r.mu.RUnlock()

	if !ok {
		return domain.Part{}, ErrNotFound
	}

	return RecordToDomain(rec)
}

// GetPartForUpdate is GetPart: the in-memory repository has no transactions
// to hold a lock for (its mutex already serializes each call).
func (r *memoryRepository) GetPartForUpdate(ctx context.Context, id uuid.UUID) (domain.Part, error) {
	return r.GetPart(ctx, id)
}

// ListParts returns every part in the catalog.
func (r *memoryRepository) ListParts(_ context.Context) ([]domain.Part, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	parts := make([]domain.Part, 0, len(r.parts))

	for _, rec := range r.parts {
		part, err := RecordToDomain(rec)
		if err != nil {
			return nil, err
		}

		parts = append(parts, part)
	}

	return parts, nil
}

// Save persists part's reserved and stock quantities.
func (r *memoryRepository) Save(_ context.Context, part domain.Part) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.parts[part.ID()]
	if !ok {
		return ErrNotFound
	}

	rec.Reserved = part.Reserved()
	rec.StockQuantity = part.StockQuantity()
	r.parts[part.ID()] = rec

	return nil
}
