// Package repository provides storage for spaceship parts.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/omaigo88/inventory/pkg/domain"
)

// ErrNotFound is returned when a part does not exist in the repository.
var ErrNotFound = errors.New("not found")

// PartRecord mirrors a row of the parts table — the persistence shape,
// converted to/from the domain.Part entity by RecordToDomain.
type PartRecord struct {
	UUID          uuid.UUID
	Name          string
	Description   string
	Price         int64
	PartType      int32
	Properties    []byte // raw JSON (the properties JSONB column's contents)
	StockQuantity int64
	Reserved      int64
	CreatedAt     time.Time
}

// Repository provides access to the parts catalog.
type Repository interface {
	GetPart(ctx context.Context, id uuid.UUID) (domain.Part, error)
	// GetPartForUpdate is GetPart plus a row lock held until the surrounding
	// transaction ends (SELECT ... FOR UPDATE).
	GetPartForUpdate(ctx context.Context, id uuid.UUID) (domain.Part, error)
	ListParts(ctx context.Context) ([]domain.Part, error)
	// Save persists a part's mutable state (reserved and stock quantity).
	Save(ctx context.Context, part domain.Part) error
}
