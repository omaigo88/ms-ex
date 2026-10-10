package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omaigo88/inventory/pkg/domain"
	"github.com/omaigo88/inventory/pkg/repository"
)

func TestRecordToDomain_Success(t *testing.T) {
	id := uuid.New()
	now := time.Now()

	rec := repository.PartRecord{
		UUID:          id,
		Name:          "Aluminum Hull",
		Description:   "desc",
		Price:         500000,
		PartType:      int32(domain.PartTypeHull),
		Properties:    []byte(`{"hull":{"mass_kg":1000,"armor_rating":20}}`),
		StockQuantity: 10,
		Reserved:      2,
		CreatedAt:     now,
	}

	part, err := repository.RecordToDomain(rec)
	require.NoError(t, err)
	assert.Equal(t, id, part.ID())
	assert.Equal(t, domain.PartTypeHull, part.Type())
	assert.Equal(t, int64(1000), part.Properties().Hull.MassKG)
	assert.Equal(t, int64(10), part.StockQuantity())
	assert.Equal(t, int64(2), part.Reserved())
}

func TestRecordToDomain_MalformedJSON(t *testing.T) {
	rec := repository.PartRecord{
		UUID:       uuid.New(),
		PartType:   int32(domain.PartTypeHull),
		Properties: []byte(`not json`),
	}

	_, err := repository.RecordToDomain(rec)
	require.Error(t, err)
}

func TestRecordToDomain_PropertiesTypeMismatch(t *testing.T) {
	rec := repository.PartRecord{
		UUID:          uuid.New(),
		PartType:      int32(domain.PartTypeEngine),
		Properties:    []byte(`{"hull":{"mass_kg":1000,"armor_rating":20}}`),
		StockQuantity: 1,
	}

	_, err := repository.RecordToDomain(rec)
	require.ErrorIs(t, err, domain.ErrInvalidProperties)
}
