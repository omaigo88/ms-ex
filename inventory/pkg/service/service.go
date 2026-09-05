// Package service contains the InventoryService business logic.
package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/omaigo88/inventory/pkg/repository"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
)

var (
	// ErrEmptyUUID is returned when a required UUID is empty.
	ErrEmptyUUID = errors.New("uuid must not be empty")
	// ErrInvalidUUID is returned when a UUID cannot be parsed.
	ErrInvalidUUID = errors.New("invalid uuid format")
	// ErrPartNotFound is returned when a part does not exist in the catalog.
	ErrPartNotFound = errors.New("part not found")
)

// Repository is the persistence dependency required by Service.
type Repository interface {
	GetPart(id uuid.UUID) (repository.Part, bool)
	ListParts() []repository.Part
}

// Service implements the InventoryService business logic.
type Service struct {
	repo Repository
}

// NewService creates a new Service backed by the given Repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// GetPart returns a part by its raw UUID string.
func (s *Service) GetPart(_ context.Context, rawUUID string) (repository.Part, error) {
	if rawUUID == "" {
		return repository.Part{}, ErrEmptyUUID
	}

	id, err := uuid.Parse(rawUUID)
	if err != nil {
		return repository.Part{}, fmt.Errorf("%w: %s", ErrInvalidUUID, rawUUID)
	}

	part, ok := s.repo.GetPart(id)
	if !ok {
		return repository.Part{}, fmt.Errorf("%w: %s", ErrPartNotFound, rawUUID)
	}

	return part, nil
}

// ListParts returns parts filtered by uuids (order preserved, partType ignored) or,
// when uuids is empty, filtered by partType (or all parts, sorted by name).
func (s *Service) ListParts(
	_ context.Context,
	partType inventoryv1.PartType,
	rawUUIDs []string,
) ([]repository.Part, error) {
	if len(rawUUIDs) > 0 {
		parts := make([]repository.Part, 0, len(rawUUIDs))

		for _, rawUUID := range rawUUIDs {
			id, err := uuid.Parse(rawUUID)
			if err != nil {
				return nil, fmt.Errorf("%w: %s", ErrInvalidUUID, rawUUID)
			}

			part, ok := s.repo.GetPart(id)
			if !ok {
				return nil, fmt.Errorf("%w: %s", ErrPartNotFound, rawUUID)
			}

			parts = append(parts, part)
		}

		return parts, nil
	}

	all := s.repo.ListParts()
	parts := make([]repository.Part, 0, len(all))

	for _, part := range all {
		if partType != inventoryv1.PartType_PART_TYPE_UNSPECIFIED && part.PartType != partType {
			continue
		}

		parts = append(parts, part)
	}

	sort.Slice(parts, func(i, j int) bool {
		return parts[i].Name < parts[j].Name
	})

	return parts, nil
}
