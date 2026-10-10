package repository

import (
	"encoding/json"
	"fmt"

	"github.com/omaigo88/inventory/pkg/domain"
)

// RecordToDomain converts a persistence PartRecord into a domain.Part,
// parsing the record's raw JSON properties into the matching Value Object.
func RecordToDomain(rec PartRecord) (domain.Part, error) {
	var props domain.PartProperties
	if err := json.Unmarshal(rec.Properties, &props); err != nil {
		return domain.Part{}, fmt.Errorf("unmarshal properties: %w", err)
	}

	return domain.NewPart(
		rec.UUID,
		rec.Name,
		rec.Description,
		rec.Price,
		domain.PartType(rec.PartType),
		props,
		rec.StockQuantity,
		rec.Reserved,
		rec.CreatedAt,
	)
}
