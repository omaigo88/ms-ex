package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/omaigo88/inventory/pkg/domain"
	"github.com/omaigo88/inventory/pkg/repository"
	"github.com/omaigo88/inventory/pkg/service"
	"github.com/omaigo88/inventory/pkg/service/mocks"
)

// mockPartWithReserved matches any domain.Part whose Reserved() equals want,
// for asserting what Save was called with without depending on which part
// in a set it was.
func mockPartWithReserved(want int64) interface{} {
	return mock.MatchedBy(func(p domain.Part) bool {
		return p.Reserved() == want
	})
}

func propertiesFor(partType domain.PartType) domain.PartProperties {
	switch partType {
	case domain.PartTypeHull:
		return domain.PartProperties{Hull: &domain.HullProperties{MassKG: 1000, ArmorRating: 20}}
	case domain.PartTypeEngine:
		return domain.PartProperties{Engine: &domain.EngineProperties{ThrustNewton: 5000, MaxHullMassKG: 1500}}
	case domain.PartTypeShield:
		return domain.PartProperties{Shield: &domain.ShieldProperties{CapacityMJ: 500}}
	case domain.PartTypeWeapon:
		return domain.PartProperties{Weapon: &domain.WeaponProperties{DamageRating: 80}}
	default:
		return domain.PartProperties{}
	}
}

func samplePart(t *testing.T, id uuid.UUID, name string, partType domain.PartType) domain.Part {
	t.Helper()

	part, err := domain.NewPart(id, name, "test part", 100, partType, propertiesFor(partType), 1, 0, time.Now())
	require.NoError(t, err)

	return part
}

func newTestService(repo service.Repository) *service.Service {
	return service.NewService(repo, domain.NewCompatibilityChecker(), service.NewNoopTrManager())
}

func TestService_GetPart_Success(t *testing.T) {
	id := uuid.New()
	part := samplePart(t, id, "Hull", domain.PartTypeHull)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPart(context.Background(), id).Return(part, nil)

	svc := newTestService(repo)

	got, err := svc.GetPart(context.Background(), id.String())
	require.NoError(t, err)
	assert.Equal(t, part, got)
}

func TestService_GetPart_EmptyUUID(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := newTestService(repo)

	_, err := svc.GetPart(context.Background(), "")
	require.ErrorIs(t, err, service.ErrEmptyUUID)
}

func TestService_GetPart_InvalidUUID(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := newTestService(repo)

	_, err := svc.GetPart(context.Background(), "not-a-uuid")
	require.ErrorIs(t, err, service.ErrInvalidUUID)
}

func TestService_GetPart_NotFound(t *testing.T) {
	id := uuid.New()

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPart(context.Background(), id).Return(domain.Part{}, repository.ErrNotFound)

	svc := newTestService(repo)

	_, err := svc.GetPart(context.Background(), id.String())
	require.ErrorIs(t, err, service.ErrPartNotFound)
}

func TestService_ListParts_ByUUIDs_PreservesOrder(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	part1 := samplePart(t, id1, "Engine", domain.PartTypeEngine)
	part2 := samplePart(t, id2, "Hull", domain.PartTypeHull)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPart(context.Background(), id1).Return(part1, nil)
	repo.EXPECT().GetPart(context.Background(), id2).Return(part2, nil)

	svc := newTestService(repo)

	parts, err := svc.ListParts(context.Background(), 0, []string{id1.String(), id2.String()})
	require.NoError(t, err)
	require.Len(t, parts, 2)
	assert.Equal(t, part1, parts[0])
	assert.Equal(t, part2, parts[1])
}

func TestService_ListParts_ByUUIDs_InvalidUUID(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := newTestService(repo)

	_, err := svc.ListParts(context.Background(), 0, []string{"not-a-uuid"})
	require.ErrorIs(t, err, service.ErrInvalidUUID)
}

func TestService_ListParts_ByUUIDs_NotFound(t *testing.T) {
	id := uuid.New()

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPart(context.Background(), id).Return(domain.Part{}, repository.ErrNotFound)

	svc := newTestService(repo)

	_, err := svc.ListParts(context.Background(), 0, []string{id.String()})
	require.ErrorIs(t, err, service.ErrPartNotFound)
}

func TestService_ListParts_ByType_FiltersAndSorts(t *testing.T) {
	hull := samplePart(t, uuid.New(), "Zeta Hull", domain.PartTypeHull)
	engine := samplePart(t, uuid.New(), "Alpha Engine", domain.PartTypeEngine)
	otherHull := samplePart(t, uuid.New(), "Alpha Hull", domain.PartTypeHull)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().ListParts(context.Background()).Return([]domain.Part{hull, engine, otherHull}, nil)

	svc := newTestService(repo)

	parts, err := svc.ListParts(context.Background(), 1, nil) // PART_TYPE_HULL == 1
	require.NoError(t, err)
	require.Len(t, parts, 2)
	assert.Equal(t, "Alpha Hull", parts[0].Name())
	assert.Equal(t, "Zeta Hull", parts[1].Name())
}

func TestService_ListParts_All_SortedByName(t *testing.T) {
	hull := samplePart(t, uuid.New(), "Zeta Hull", domain.PartTypeHull)
	engine := samplePart(t, uuid.New(), "Alpha Engine", domain.PartTypeEngine)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().ListParts(context.Background()).Return([]domain.Part{hull, engine}, nil)

	svc := newTestService(repo)

	parts, err := svc.ListParts(context.Background(), 0, nil)
	require.NoError(t, err)
	require.Len(t, parts, 2)
	assert.Equal(t, "Alpha Engine", parts[0].Name())
	assert.Equal(t, "Zeta Hull", parts[1].Name())
}

func TestService_ReserveParts_Success(t *testing.T) {
	hullID, engineID := uuid.New(), uuid.New()
	hull := samplePart(t, hullID, "Hull", domain.PartTypeHull)
	engine := samplePart(t, engineID, "Engine", domain.PartTypeEngine)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPartForUpdate(context.Background(), hullID).Return(hull, nil)
	repo.EXPECT().GetPartForUpdate(context.Background(), engineID).Return(engine, nil)
	repo.EXPECT().Save(context.Background(), mockPartWithReserved(1)).Return(nil).Times(2)

	svc := newTestService(repo)

	err := svc.ReserveParts(context.Background(), []string{hullID.String(), engineID.String()})
	require.NoError(t, err)
}

func TestService_ReserveParts_NotFound(t *testing.T) {
	id := uuid.New()

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPartForUpdate(context.Background(), id).Return(domain.Part{}, repository.ErrNotFound)

	svc := newTestService(repo)

	err := svc.ReserveParts(context.Background(), []string{id.String()})
	require.ErrorIs(t, err, service.ErrPartNotFound)
}

func TestService_ReserveParts_InsufficientStock(t *testing.T) {
	id := uuid.New()
	part, err := domain.NewPart(id, "Hull", "d", 1, domain.PartTypeHull, propertiesFor(domain.PartTypeHull), 1, 1, time.Now())
	require.NoError(t, err)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPartForUpdate(context.Background(), id).Return(part, nil)

	svc := newTestService(repo)

	err = svc.ReserveParts(context.Background(), []string{id.String()})
	require.ErrorIs(t, err, domain.ErrInsufficientStock)
}

func TestService_ReserveParts_Incompatible(t *testing.T) {
	hullID, engineID := uuid.New(), uuid.New()
	hull, err := domain.NewPart(hullID, "Hull", "d", 1, domain.PartTypeHull,
		domain.PartProperties{Hull: &domain.HullProperties{MassKG: 3000}}, 1, 0, time.Now())
	require.NoError(t, err)

	engine, err := domain.NewPart(engineID, "Engine", "d", 1, domain.PartTypeEngine,
		domain.PartProperties{Engine: &domain.EngineProperties{MaxHullMassKG: 1500}}, 1, 0, time.Now())
	require.NoError(t, err)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPartForUpdate(context.Background(), hullID).Return(hull, nil)
	repo.EXPECT().GetPartForUpdate(context.Background(), engineID).Return(engine, nil)

	svc := newTestService(repo)

	err = svc.ReserveParts(context.Background(), []string{hullID.String(), engineID.String()})
	require.ErrorIs(t, err, domain.ErrIncompatibleParts)
}

func TestService_ReleaseParts_Success(t *testing.T) {
	id := uuid.New()
	part, err := domain.NewPart(id, "Hull", "d", 1, domain.PartTypeHull, propertiesFor(domain.PartTypeHull), 10, 1, time.Now())
	require.NoError(t, err)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPartForUpdate(context.Background(), id).Return(part, nil)
	repo.EXPECT().Save(context.Background(), mockPartWithReserved(0)).Return(nil)

	svc := newTestService(repo)

	err = svc.ReleaseParts(context.Background(), []string{id.String()})
	require.NoError(t, err)
}

func TestService_ReleaseParts_Underflow(t *testing.T) {
	id := uuid.New()
	part, err := domain.NewPart(id, "Hull", "d", 1, domain.PartTypeHull, propertiesFor(domain.PartTypeHull), 10, 0, time.Now())
	require.NoError(t, err)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPartForUpdate(context.Background(), id).Return(part, nil)

	svc := newTestService(repo)

	err = svc.ReleaseParts(context.Background(), []string{id.String()})
	require.ErrorIs(t, err, domain.ErrReservationUnderflow)
}

func TestService_CommitParts_Success(t *testing.T) {
	id := uuid.New()
	part, err := domain.NewPart(id, "Hull", "d", 1, domain.PartTypeHull, propertiesFor(domain.PartTypeHull), 10, 1, time.Now())
	require.NoError(t, err)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPartForUpdate(context.Background(), id).Return(part, nil)
	repo.EXPECT().Save(context.Background(), mock.MatchedBy(func(p domain.Part) bool {
		return p.Reserved() == 0 && p.StockQuantity() == 9
	})).Return(nil)

	svc := newTestService(repo)

	require.NoError(t, svc.CommitParts(context.Background(), []string{id.String()}))
}

func TestService_CommitParts_NothingReserved(t *testing.T) {
	id := uuid.New()
	part, err := domain.NewPart(id, "Hull", "d", 1, domain.PartTypeHull, propertiesFor(domain.PartTypeHull), 10, 0, time.Now())
	require.NoError(t, err)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPartForUpdate(context.Background(), id).Return(part, nil)

	svc := newTestService(repo)

	err = svc.CommitParts(context.Background(), []string{id.String()})
	require.ErrorIs(t, err, domain.ErrReservationUnderflow)
}

func TestService_CommitParts_InvalidUUID(t *testing.T) {
	svc := newTestService(mocks.NewMockRepository(t))

	err := svc.CommitParts(context.Background(), []string{"not-a-uuid"})
	require.ErrorIs(t, err, service.ErrInvalidUUID)
}

// TestService_ReserveParts_LocksInSortedOrder guards the deadlock-avoidance
// rule: rows are locked in UUID order regardless of request order.
func TestService_ReserveParts_LocksInSortedOrder(t *testing.T) {
	low := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	high := uuid.MustParse("ffffffff-0000-4000-8000-000000000001")

	var locked []uuid.UUID

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPartForUpdate(context.Background(), mock.Anything).
		RunAndReturn(func(_ context.Context, id uuid.UUID) (domain.Part, error) {
			locked = append(locked, id)
			partType := domain.PartTypeShield
			if id == low {
				partType = domain.PartTypeWeapon
			}

			return samplePart(t, id, "p", partType), nil
		}).Times(2)
	repo.EXPECT().Save(context.Background(), mock.Anything).Return(nil).Times(2)

	svc := newTestService(repo)

	require.NoError(t, svc.ReserveParts(context.Background(), []string{high.String(), low.String()}))
	assert.Equal(t, []uuid.UUID{low, high}, locked)
}
