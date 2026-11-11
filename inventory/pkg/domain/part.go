// Package domain contains the InventoryService domain model: the rich Part
// entity, its Value Objects, and domain services. Nothing here depends on
// proto-generated or persistence types.
package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Part is a rich entity representing a spaceship part in the catalog.
type Part struct {
	id            uuid.UUID
	name          string
	description   string
	price         int64
	partType      PartType
	properties    PartProperties
	stockQuantity int64
	reserved      int64
	createdAt     time.Time
}

// NewPart constructs a Part, validating that properties matches partType and
// that the stock/reserved invariants hold.
func NewPart(
	id uuid.UUID,
	name string,
	description string,
	price int64,
	partType PartType,
	properties PartProperties,
	stockQuantity int64,
	reserved int64,
	createdAt time.Time,
) (Part, error) {
	if !properties.matchesType(partType) {
		return Part{}, fmt.Errorf("%w: part %s has type %d", ErrInvalidProperties, id, partType)
	}

	if stockQuantity < 0 {
		return Part{}, fmt.Errorf("%w: stock_quantity must not be negative", ErrInvalidQuantity)
	}

	if reserved < 0 || reserved > stockQuantity {
		return Part{}, fmt.Errorf("%w: reserved must be between 0 and stock_quantity", ErrInvalidQuantity)
	}

	return Part{
		id:            id,
		name:          name,
		description:   description,
		price:         price,
		partType:      partType,
		properties:    properties,
		stockQuantity: stockQuantity,
		reserved:      reserved,
		createdAt:     createdAt,
	}, nil
}

func (p Part) ID() uuid.UUID              { return p.id }
func (p Part) Name() string               { return p.name }
func (p Part) Description() string        { return p.description }
func (p Part) Price() int64               { return p.price }
func (p Part) Type() PartType             { return p.partType }
func (p Part) Properties() PartProperties { return p.properties }
func (p Part) StockQuantity() int64       { return p.stockQuantity }
func (p Part) Reserved() int64            { return p.reserved }
func (p Part) CreatedAt() time.Time       { return p.createdAt }

// Available returns how many units of this part aren't currently reserved.
func (p Part) Available() int64 {
	return p.stockQuantity - p.reserved
}

// Reserve holds qty units of this part against its available stock.
func (p *Part) Reserve(qty int64) error {
	if qty <= 0 {
		return ErrInvalidQuantity
	}

	if p.Available() < qty {
		return fmt.Errorf("%w: part %s", ErrInsufficientStock, p.id)
	}

	p.reserved += qty

	return nil
}

// Release frees qty previously-reserved units of this part.
func (p *Part) Release(qty int64) error {
	if qty <= 0 {
		return ErrInvalidQuantity
	}

	if p.reserved < qty {
		return fmt.Errorf("%w: part %s", ErrReservationUnderflow, p.id)
	}

	p.reserved -= qty

	return nil
}

// Commit finalizes qty previously-reserved units: they leave the warehouse,
// so both the reservation and the stock quantity go down.
func (p *Part) Commit(qty int64) error {
	if qty <= 0 {
		return ErrInvalidQuantity
	}

	if p.reserved < qty {
		return fmt.Errorf("%w: part %s", ErrReservationUnderflow, p.id)
	}

	p.reserved -= qty
	p.stockQuantity -= qty

	return nil
}
