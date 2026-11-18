package auth_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/omaigo88/shared/pkg/auth"
)

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()

	_, ok := auth.SessionUUIDFromContext(ctx)
	assert.False(t, ok)

	_, ok = auth.UserUUIDFromContext(ctx)
	assert.False(t, ok)

	userUUID := uuid.New()
	ctx = auth.WithUserUUID(auth.WithSessionUUID(ctx, "session-1"), userUUID)

	session, ok := auth.SessionUUIDFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "session-1", session)

	got, ok := auth.UserUUIDFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, userUUID, got)
}

func TestSessionUUIDFromContext_EmptyIsAbsent(t *testing.T) {
	_, ok := auth.SessionUUIDFromContext(auth.WithSessionUUID(context.Background(), ""))
	assert.False(t, ok)
}

// outgoingSession runs the interceptor and returns the session-uuid values the
// invoker saw in outgoing metadata.
func outgoingSession(t *testing.T, ctx context.Context) []string {
	t.Helper()

	var got []string

	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		got = md.Get(auth.SessionMetadataKey)

		return nil
	}

	require.NoError(t, auth.UnaryClientInterceptor()(ctx, "/svc/Method", nil, nil, nil, invoker))

	return got
}

func TestUnaryClientInterceptor_ForwardsSession(t *testing.T) {
	ctx := auth.WithSessionUUID(context.Background(), "session-1")

	assert.Equal(t, []string{"session-1"}, outgoingSession(t, ctx))
}

func TestUnaryClientInterceptor_NoSessionNoMetadata(t *testing.T) {
	assert.Empty(t, outgoingSession(t, context.Background()))
}
