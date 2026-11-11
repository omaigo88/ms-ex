// Package service contains the InventoryService business logic.
package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/omaigo88/inventory/pkg/domain"
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
	GetPart(ctx context.Context, id uuid.UUID) (domain.Part, error)
	GetPartForUpdate(ctx context.Context, id uuid.UUID) (domain.Part, error)
	ListParts(ctx context.Context) ([]domain.Part, error)
	Save(ctx context.Context, part domain.Part) error
}

// Service implements the InventoryService business logic.
type Service struct {
	repo      Repository
	checker   *domain.CompatibilityChecker
	trManager TrManager
}

// NewService creates a new Service backed by the given Repository.
func NewService(repo Repository, checker *domain.CompatibilityChecker, trManager TrManager) *Service {
	return &Service{repo: repo, checker: checker, trManager: trManager}
}

// GetPart returns a part by its raw UUID string.
func (s *Service) GetPart(ctx context.Context, rawUUID string) (domain.Part, error) {
	if rawUUID == "" {
		return domain.Part{}, ErrEmptyUUID
	}

	id, err := uuid.Parse(rawUUID)
	if err != nil {
		return domain.Part{}, fmt.Errorf("%w: %s", ErrInvalidUUID, rawUUID)
	}

	part, err := s.repo.GetPart(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return domain.Part{}, fmt.Errorf("%w: %s", ErrPartNotFound, rawUUID)
	}

	if err != nil {
		return domain.Part{}, err
	}

	return part, nil
}

// ListParts returns parts filtered by uuids (order preserved, partType ignored) or,
// when uuids is empty, filtered by partType (or all parts, sorted by name).
func (s *Service) ListParts(
	ctx context.Context,
	partType inventoryv1.PartType,
	rawUUIDs []string,
) ([]domain.Part, error) {
	if len(rawUUIDs) > 0 {
		parts := make([]domain.Part, 0, len(rawUUIDs))

		for _, rawUUID := range rawUUIDs {
			id, err := uuid.Parse(rawUUID)
			if err != nil {
				return nil, fmt.Errorf("%w: %s", ErrInvalidUUID, rawUUID)
			}

			part, err := s.repo.GetPart(ctx, id)
			if errors.Is(err, repository.ErrNotFound) {
				return nil, fmt.Errorf("%w: %s", ErrPartNotFound, rawUUID)
			}

			if err != nil {
				return nil, err
			}

			parts = append(parts, part)
		}

		return parts, nil
	}

	all, err := s.repo.ListParts(ctx)
	if err != nil {
		return nil, err
	}

	wantType := domain.PartType(partType)
	parts := make([]domain.Part, 0, len(all))

	for _, part := range all {
		if wantType != domain.PartTypeUnspecified && part.Type() != wantType {
			continue
		}

		parts = append(parts, part)
	}

	sort.Slice(parts, func(i, j int) bool {
		return parts[i].Name() < parts[j].Name()
	})

	return parts, nil
}

// lockParts parses every raw UUID and fetches its part with a row lock. The
// UUIDs are locked in sorted order, so concurrent requests over overlapping
// parts always acquire locks in the same order and can't deadlock. Must be
// called inside a transaction for the locks to mean anything.
func (s *Service) lockParts(ctx context.Context, rawUUIDs []string) ([]domain.Part, error) {
	ids := make([]uuid.UUID, 0, len(rawUUIDs))

	for _, rawUUID := range rawUUIDs {
		id, err := uuid.Parse(rawUUID)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidUUID, rawUUID)
		}

		ids = append(ids, id)
	}

	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})

	parts := make([]domain.Part, 0, len(ids))

	for _, id := range ids {
		part, err := s.repo.GetPartForUpdate(ctx, id)
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrPartNotFound, id)
		}

		if err != nil {
			return nil, err
		}

		parts = append(parts, part)
	}

	return parts, nil
}

// updateParts locks every listed part, applies mutate to each, runs check
// (if any) over the mutated set, and persists — all in one transaction, so
// either every part is updated or none is.
func (s *Service) updateParts(
	ctx context.Context,
	rawUUIDs []string,
	mutate func(p *domain.Part) error,
	check func(parts []domain.Part) error,
) error {
	return s.trManager.Do(ctx, func(ctx context.Context) error {
		parts, err := s.lockParts(ctx, rawUUIDs)
		if err != nil {
			return err
		}

		for i := range parts {
			if err := mutate(&parts[i]); err != nil {
				return err
			}
		}

		if check != nil {
			if err := check(parts); err != nil {
				return err
			}
		}

		for i := range parts {
			if err := s.repo.Save(ctx, parts[i]); err != nil {
				return err
			}
		}

		return nil
	})
}

// ReserveParts reserves exactly one unit of every listed part, atomically as
// a set: stock and cross-part compatibility are both checked before any
// reservation is persisted.
func (s *Service) ReserveParts(ctx context.Context, rawUUIDs []string) error {
	return s.updateParts(
		ctx, rawUUIDs,
		func(p *domain.Part) error { return p.Reserve(1) },
		s.checker.Check,
	)
}

// ReleaseParts releases a previous reservation of one unit of every listed part.
func (s *Service) ReleaseParts(ctx context.Context, rawUUIDs []string) error {
	return s.updateParts(
		ctx, rawUUIDs,
		func(p *domain.Part) error { return p.Release(1) },
		nil,
	)
}

// CommitParts writes off one previously-reserved unit of every listed part:
// it leaves the warehouse after the ship has been assembled.
func (s *Service) CommitParts(ctx context.Context, rawUUIDs []string) error {
	return s.updateParts(
		ctx, rawUUIDs,
		func(p *domain.Part) error { return p.Commit(1) },
		nil,
	)
}
