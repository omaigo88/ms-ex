package api_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/omaigo88/inventory/pkg/api"
	"github.com/omaigo88/inventory/pkg/api/mocks"
	"github.com/omaigo88/shared/pkg/auth"
	authv1 "github.com/omaigo88/shared/pkg/proto/auth/v1"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
)

// intercept runs the interceptor for method and returns the context the
// handler saw (nil if it wasn't reached) and the interceptor's error.
func intercept(t *testing.T, iam api.IAMClient, ctx context.Context, method string) (context.Context, error) {
	t.Helper()

	var reached context.Context

	handler := func(ctx context.Context, _ any) (any, error) {
		reached = ctx

		return "ok", nil
	}

	_, err := api.AuthInterceptor(iam)(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, handler)

	return reached, err
}

func withSession(session string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(auth.SessionMetadataKey, session))
}

func TestAuthInterceptor_ValidSession(t *testing.T) {
	userUUID := uuid.New()

	iam := mocks.NewMockIAMClient(t)
	iam.EXPECT().Whoami(mock.Anything, &authv1.WhoamiRequest{SessionUuid: "session-1"}).
		Return(&authv1.WhoamiResponse{UserUuid: userUUID.String()}, nil)

	ctx, err := intercept(t, iam, withSession("session-1"), inventoryv1.InventoryService_ListParts_FullMethodName)
	require.NoError(t, err)
	require.NotNil(t, ctx)

	got, ok := auth.UserUUIDFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, userUUID, got)

	session, ok := auth.SessionUUIDFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "session-1", session)
}

func TestAuthInterceptor_MissingSession(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"no metadata":   context.Background(),
		"empty session": withSession(""),
	} {
		t.Run(name, func(t *testing.T) {
			reached, err := intercept(t, mocks.NewMockIAMClient(t), ctx, inventoryv1.InventoryService_ReserveParts_FullMethodName)
			assert.Nil(t, reached)
			assert.Equal(t, codes.Unauthenticated, status.Code(err))
		})
	}
}

func TestAuthInterceptor_RejectedSession(t *testing.T) {
	iam := mocks.NewMockIAMClient(t)
	iam.EXPECT().Whoami(mock.Anything, mock.Anything).Return(nil, status.Error(codes.Unauthenticated, "session not found"))

	reached, err := intercept(t, iam, withSession("expired"), inventoryv1.InventoryService_GetPart_FullMethodName)
	assert.Nil(t, reached)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestAuthInterceptor_IAMUnavailable(t *testing.T) {
	iam := mocks.NewMockIAMClient(t)
	iam.EXPECT().Whoami(mock.Anything, mock.Anything).Return(nil, status.Error(codes.Unavailable, "down"))

	reached, err := intercept(t, iam, withSession("session-1"), inventoryv1.InventoryService_GetPart_FullMethodName)
	assert.Nil(t, reached)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestAuthInterceptor_CommitPartsIsExempt(t *testing.T) {
	// No IAM expectations: CommitParts comes from the Kafka path without a session.
	reached, err := intercept(t, mocks.NewMockIAMClient(t), context.Background(), inventoryv1.InventoryService_CommitParts_FullMethodName)
	require.NoError(t, err)
	assert.NotNil(t, reached)
}
