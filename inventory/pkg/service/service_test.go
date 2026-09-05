package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/omaigo88/inventory/pkg/repository"
	"github.com/omaigo88/inventory/pkg/service"
	"github.com/omaigo88/inventory/pkg/service/mocks"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
)

func samplePart(id uuid.UUID, name string, partType inventoryv1.PartType) repository.Part {
	return repository.Part{
		UUID:          id.String(),
		Name:          name,
		Description:   "test part",
		Price:         100,
		PartType:      partType,
		StockQuantity: 1,
		CreatedAt:     timestamppb.Now(),
	}
}

func TestService_GetPart_Success(t *testing.T) {
	id := uuid.New()
	part := samplePart(id, "Hull", inventoryv1.PartType_PART_TYPE_HULL)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPart(id).Return(part, true)

	svc := service.NewService(repo)

	got, err := svc.GetPart(context.Background(), id.String())
	require.NoError(t, err)
	assert.Equal(t, part, got)
}

func TestService_GetPart_EmptyUUID(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := service.NewService(repo)

	_, err := svc.GetPart(context.Background(), "")
	require.ErrorIs(t, err, service.ErrEmptyUUID)
}

func TestService_GetPart_InvalidUUID(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := service.NewService(repo)

	_, err := svc.GetPart(context.Background(), "not-a-uuid")
	require.ErrorIs(t, err, service.ErrInvalidUUID)
}

func TestService_GetPart_NotFound(t *testing.T) {
	id := uuid.New()

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPart(id).Return(repository.Part{}, false)

	svc := service.NewService(repo)

	_, err := svc.GetPart(context.Background(), id.String())
	require.ErrorIs(t, err, service.ErrPartNotFound)
}

func TestService_ListParts_ByUUIDs_PreservesOrder(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	part1 := samplePart(id1, "Engine", inventoryv1.PartType_PART_TYPE_ENGINE)
	part2 := samplePart(id2, "Hull", inventoryv1.PartType_PART_TYPE_HULL)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPart(id1).Return(part1, true)
	repo.EXPECT().GetPart(id2).Return(part2, true)

	svc := service.NewService(repo)

	parts, err := svc.ListParts(context.Background(), inventoryv1.PartType_PART_TYPE_UNSPECIFIED, []string{id1.String(), id2.String()})
	require.NoError(t, err)
	require.Len(t, parts, 2)
	assert.Equal(t, part1, parts[0])
	assert.Equal(t, part2, parts[1])
}

func TestService_ListParts_ByUUIDs_InvalidUUID(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := service.NewService(repo)

	_, err := svc.ListParts(context.Background(), inventoryv1.PartType_PART_TYPE_UNSPECIFIED, []string{"not-a-uuid"})
	require.ErrorIs(t, err, service.ErrInvalidUUID)
}

func TestService_ListParts_ByUUIDs_NotFound(t *testing.T) {
	id := uuid.New()

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetPart(id).Return(repository.Part{}, false)

	svc := service.NewService(repo)

	_, err := svc.ListParts(context.Background(), inventoryv1.PartType_PART_TYPE_UNSPECIFIED, []string{id.String()})
	require.ErrorIs(t, err, service.ErrPartNotFound)
}

func TestService_ListParts_ByType_FiltersAndSorts(t *testing.T) {
	hull := samplePart(uuid.New(), "Zeta Hull", inventoryv1.PartType_PART_TYPE_HULL)
	engine := samplePart(uuid.New(), "Alpha Engine", inventoryv1.PartType_PART_TYPE_ENGINE)
	otherHull := samplePart(uuid.New(), "Alpha Hull", inventoryv1.PartType_PART_TYPE_HULL)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().ListParts().Return([]repository.Part{hull, engine, otherHull})

	svc := service.NewService(repo)

	parts, err := svc.ListParts(context.Background(), inventoryv1.PartType_PART_TYPE_HULL, nil)
	require.NoError(t, err)
	require.Len(t, parts, 2)
	assert.Equal(t, "Alpha Hull", parts[0].Name)
	assert.Equal(t, "Zeta Hull", parts[1].Name)
}

func TestService_ListParts_All_SortedByName(t *testing.T) {
	hull := samplePart(uuid.New(), "Zeta Hull", inventoryv1.PartType_PART_TYPE_HULL)
	engine := samplePart(uuid.New(), "Alpha Engine", inventoryv1.PartType_PART_TYPE_ENGINE)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().ListParts().Return([]repository.Part{hull, engine})

	svc := service.NewService(repo)

	parts, err := svc.ListParts(context.Background(), inventoryv1.PartType_PART_TYPE_UNSPECIFIED, nil)
	require.NoError(t, err)
	require.Len(t, parts, 2)
	assert.Equal(t, "Alpha Engine", parts[0].Name)
	assert.Equal(t, "Zeta Hull", parts[1].Name)
}
