package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omaigo88/inventory/pkg/domain"
)

func hullProps() domain.PartProperties {
	return domain.PartProperties{Hull: &domain.HullProperties{MassKG: 1000, ArmorRating: 20}}
}

func TestNewPart_Success(t *testing.T) {
	id := uuid.New()
	now := time.Now()

	part, err := domain.NewPart(id, "Aluminum Hull", "desc", 500000, domain.PartTypeHull, hullProps(), 10, 2, now)
	require.NoError(t, err)
	assert.Equal(t, id, part.ID())
	assert.Equal(t, "Aluminum Hull", part.Name())
	assert.Equal(t, int64(500000), part.Price())
	assert.Equal(t, domain.PartTypeHull, part.Type())
	assert.Equal(t, int64(10), part.StockQuantity())
	assert.Equal(t, int64(2), part.Reserved())
	assert.Equal(t, int64(8), part.Available())
	assert.Equal(t, now, part.CreatedAt())
}

func TestNewPart_PropertiesMismatch(t *testing.T) {
	_, err := domain.NewPart(uuid.New(), "n", "d", 1, domain.PartTypeEngine, hullProps(), 1, 0, time.Now())
	require.ErrorIs(t, err, domain.ErrInvalidProperties)
}

func TestNewPart_NegativeStock(t *testing.T) {
	_, err := domain.NewPart(uuid.New(), "n", "d", 1, domain.PartTypeHull, hullProps(), -1, 0, time.Now())
	require.ErrorIs(t, err, domain.ErrInvalidQuantity)
}

func TestNewPart_ReservedExceedsStock(t *testing.T) {
	_, err := domain.NewPart(uuid.New(), "n", "d", 1, domain.PartTypeHull, hullProps(), 5, 6, time.Now())
	require.ErrorIs(t, err, domain.ErrInvalidQuantity)
}

func TestNewPart_ReservedNegative(t *testing.T) {
	_, err := domain.NewPart(uuid.New(), "n", "d", 1, domain.PartTypeHull, hullProps(), 5, -1, time.Now())
	require.ErrorIs(t, err, domain.ErrInvalidQuantity)
}

func TestPart_Reserve_Success(t *testing.T) {
	part, err := domain.NewPart(uuid.New(), "n", "d", 1, domain.PartTypeHull, hullProps(), 10, 0, time.Now())
	require.NoError(t, err)

	require.NoError(t, part.Reserve(3))
	assert.Equal(t, int64(3), part.Reserved())
	assert.Equal(t, int64(7), part.Available())
}

func TestPart_Reserve_InsufficientStock(t *testing.T) {
	part, err := domain.NewPart(uuid.New(), "n", "d", 1, domain.PartTypeHull, hullProps(), 2, 0, time.Now())
	require.NoError(t, err)

	require.ErrorIs(t, part.Reserve(3), domain.ErrInsufficientStock)
	assert.Equal(t, int64(0), part.Reserved())
}

func TestPart_Reserve_InvalidQuantity(t *testing.T) {
	part, err := domain.NewPart(uuid.New(), "n", "d", 1, domain.PartTypeHull, hullProps(), 10, 0, time.Now())
	require.NoError(t, err)

	require.ErrorIs(t, part.Reserve(0), domain.ErrInvalidQuantity)
	require.ErrorIs(t, part.Reserve(-1), domain.ErrInvalidQuantity)
}

func TestPart_Release_Success(t *testing.T) {
	part, err := domain.NewPart(uuid.New(), "n", "d", 1, domain.PartTypeHull, hullProps(), 10, 4, time.Now())
	require.NoError(t, err)

	require.NoError(t, part.Release(4))
	assert.Equal(t, int64(0), part.Reserved())
	assert.Equal(t, int64(10), part.Available())
}

func TestPart_Release_Underflow(t *testing.T) {
	part, err := domain.NewPart(uuid.New(), "n", "d", 1, domain.PartTypeHull, hullProps(), 10, 1, time.Now())
	require.NoError(t, err)

	require.ErrorIs(t, part.Release(2), domain.ErrReservationUnderflow)
	assert.Equal(t, int64(1), part.Reserved())
}

func TestPart_Release_InvalidQuantity(t *testing.T) {
	part, err := domain.NewPart(uuid.New(), "n", "d", 1, domain.PartTypeHull, hullProps(), 10, 1, time.Now())
	require.NoError(t, err)

	require.ErrorIs(t, part.Release(0), domain.ErrInvalidQuantity)
}
