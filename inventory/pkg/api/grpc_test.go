package api_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/omaigo88/inventory/pkg/api"
	"github.com/omaigo88/inventory/pkg/api/mocks"
	"github.com/omaigo88/inventory/pkg/repository"
	"github.com/omaigo88/inventory/pkg/service"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
)

func TestServer_GetPart_Success(t *testing.T) {
	id := uuid.New()
	part := repository.Part{
		UUID:          id.String(),
		Name:          "Hull",
		Price:         100,
		PartType:      inventoryv1.PartType_PART_TYPE_HULL,
		StockQuantity: 1,
	}

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
		Return(repository.Part{}, fmt.Errorf("%w: %s", service.ErrPartNotFound, id))

	server := api.NewServer(svc)

	_, err := server.GetPart(context.Background(), &inventoryv1.GetPartRequest{Uuid: id.String()})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestServer_GetPart_InvalidArgument(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().GetPart(context.Background(), "").Return(repository.Part{}, service.ErrEmptyUUID)

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
	svc.EXPECT().GetPart(context.Background(), id.String()).Return(repository.Part{}, errors.New("boom"))

	server := api.NewServer(svc)

	_, err := server.GetPart(context.Background(), &inventoryv1.GetPartRequest{Uuid: id.String()})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}

func TestServer_ListParts_Success(t *testing.T) {
	parts := []repository.Part{
		{UUID: uuid.New().String(), Name: "Hull", PartType: inventoryv1.PartType_PART_TYPE_HULL},
		{UUID: uuid.New().String(), Name: "Engine", PartType: inventoryv1.PartType_PART_TYPE_ENGINE},
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
