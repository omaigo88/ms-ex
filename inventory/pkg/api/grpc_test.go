package api_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/omaigo88/inventory/pkg/api"
	"github.com/omaigo88/inventory/pkg/api/mocks"
	"github.com/omaigo88/inventory/pkg/domain"
	"github.com/omaigo88/inventory/pkg/service"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
)

func samplePart(t *testing.T, id uuid.UUID, name string) domain.Part {
	t.Helper()

	props := domain.PartProperties{Hull: &domain.HullProperties{MassKG: 1000, ArmorRating: 20}}
	part, err := domain.NewPart(id, name, "desc", 100, domain.PartTypeHull, props, 1, 0, time.Now())
	require.NoError(t, err)

	return part
}

func TestServer_GetPart_Success(t *testing.T) {
	id := uuid.New()
	part := samplePart(t, id, "Hull")

	svc := mocks.NewMockService(t)
	svc.EXPECT().GetPart(context.Background(), id.String()).Return(part, nil)

	server := api.NewServer(svc)

	resp, err := server.GetPart(context.Background(), &inventoryv1.GetPartRequest{Uuid: id.String()})
	require.NoError(t, err)
	assert.Equal(t, id.String(), resp.GetPart().GetUuid())
	assert.Equal(t, "Hull", resp.GetPart().GetName())
}

func TestServer_GetPart_NotFound(t *testing.T) {
	id := uuid.New()

	svc := mocks.NewMockService(t)
	svc.EXPECT().GetPart(context.Background(), id.String()).
		Return(domain.Part{}, fmt.Errorf("%w: %s", service.ErrPartNotFound, id))

	server := api.NewServer(svc)

	_, err := server.GetPart(context.Background(), &inventoryv1.GetPartRequest{Uuid: id.String()})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestServer_GetPart_InvalidArgument(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().GetPart(context.Background(), "").Return(domain.Part{}, service.ErrEmptyUUID)

	server := api.NewServer(svc)

	_, err := server.GetPart(context.Background(), &inventoryv1.GetPartRequest{Uuid: ""})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestServer_GetPart_InternalError(t *testing.T) {
	id := uuid.New()

	svc := mocks.NewMockService(t)
	svc.EXPECT().GetPart(context.Background(), id.String()).Return(domain.Part{}, errors.New("boom"))

	server := api.NewServer(svc)

	_, err := server.GetPart(context.Background(), &inventoryv1.GetPartRequest{Uuid: id.String()})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}

func TestServer_ListParts_Success(t *testing.T) {
	parts := []domain.Part{
		samplePart(t, uuid.New(), "Hull"),
		samplePart(t, uuid.New(), "Engine"),
	}

	svc := mocks.NewMockService(t)
	svc.EXPECT().
		ListParts(context.Background(), inventoryv1.PartType_PART_TYPE_UNSPECIFIED, []string(nil)).
		Return(parts, nil)

	server := api.NewServer(svc)

	resp, err := server.ListParts(context.Background(), &inventoryv1.ListPartsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 2)
}

func TestServer_ListParts_Error(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().
		ListParts(context.Background(), inventoryv1.PartType_PART_TYPE_UNSPECIFIED, []string{"bad"}).
		Return(nil, service.ErrInvalidUUID)

	server := api.NewServer(svc)

	_, err := server.ListParts(context.Background(), &inventoryv1.ListPartsRequest{Uuids: []string{"bad"}})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestServer_ReserveParts_Success(t *testing.T) {
	uuids := []string{uuid.New().String(), uuid.New().String()}

	svc := mocks.NewMockService(t)
	svc.EXPECT().ReserveParts(context.Background(), uuids).Return(nil)

	server := api.NewServer(svc)

	_, err := server.ReserveParts(context.Background(), &inventoryv1.ReservePartsRequest{Uuids: uuids})
	require.NoError(t, err)
}

func TestServer_ReserveParts_InsufficientStock(t *testing.T) {
	uuids := []string{uuid.New().String()}

	svc := mocks.NewMockService(t)
	svc.EXPECT().ReserveParts(context.Background(), uuids).Return(domain.ErrInsufficientStock)

	server := api.NewServer(svc)

	_, err := server.ReserveParts(context.Background(), &inventoryv1.ReservePartsRequest{Uuids: uuids})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
}

func TestServer_ReserveParts_Incompatible(t *testing.T) {
	uuids := []string{uuid.New().String(), uuid.New().String()}

	svc := mocks.NewMockService(t)
	svc.EXPECT().ReserveParts(context.Background(), uuids).Return(domain.ErrIncompatibleParts)

	server := api.NewServer(svc)

	_, err := server.ReserveParts(context.Background(), &inventoryv1.ReservePartsRequest{Uuids: uuids})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
}

func TestServer_ReserveParts_NotFound(t *testing.T) {
	id := uuid.New()
	uuids := []string{id.String()}

	svc := mocks.NewMockService(t)
	svc.EXPECT().ReserveParts(context.Background(), uuids).
		Return(fmt.Errorf("%w: %s", service.ErrPartNotFound, id))

	server := api.NewServer(svc)

	_, err := server.ReserveParts(context.Background(), &inventoryv1.ReservePartsRequest{Uuids: uuids})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestServer_ReleaseParts_Success(t *testing.T) {
	uuids := []string{uuid.New().String()}

	svc := mocks.NewMockService(t)
	svc.EXPECT().ReleaseParts(context.Background(), uuids).Return(nil)

	server := api.NewServer(svc)

	_, err := server.ReleaseParts(context.Background(), &inventoryv1.ReleasePartsRequest{Uuids: uuids})
	require.NoError(t, err)
}

func TestServer_ReleaseParts_Underflow(t *testing.T) {
	uuids := []string{uuid.New().String()}

	svc := mocks.NewMockService(t)
	svc.EXPECT().ReleaseParts(context.Background(), uuids).Return(domain.ErrReservationUnderflow)

	server := api.NewServer(svc)

	_, err := server.ReleaseParts(context.Background(), &inventoryv1.ReleasePartsRequest{Uuids: uuids})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
}

func TestServer_CommitParts_Success(t *testing.T) {
	uuids := []string{uuid.New().String()}

	svc := mocks.NewMockService(t)
	svc.EXPECT().CommitParts(context.Background(), uuids).Return(nil)

	server := api.NewServer(svc)

	_, err := server.CommitParts(context.Background(), &inventoryv1.CommitPartsRequest{Uuids: uuids})
	require.NoError(t, err)
}

func TestServer_CommitParts_Underflow(t *testing.T) {
	uuids := []string{uuid.New().String()}

	svc := mocks.NewMockService(t)
	svc.EXPECT().CommitParts(context.Background(), uuids).Return(domain.ErrReservationUnderflow)

	server := api.NewServer(svc)

	_, err := server.CommitParts(context.Background(), &inventoryv1.CommitPartsRequest{Uuids: uuids})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
}
