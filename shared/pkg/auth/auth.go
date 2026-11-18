// Package auth carries the authenticated session between layers and services:
// context helpers for the session and its user, and a gRPC client interceptor
// that forwards the session to downstream services via outgoing metadata.
package auth

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// SessionMetadataKey is the gRPC metadata key carrying the session UUID
// between services (gRPC lowercases metadata keys).
const SessionMetadataKey = "session-uuid"

type ctxKey int

const (
	sessionUUIDKey ctxKey = iota
	userUUIDKey
)

// WithSessionUUID returns a copy of ctx carrying the session UUID.
func WithSessionUUID(ctx context.Context, sessionUUID string) context.Context {
	return context.WithValue(ctx, sessionUUIDKey, sessionUUID)
}

// SessionUUIDFromContext returns the session UUID stored by WithSessionUUID.
func SessionUUIDFromContext(ctx context.Context) (string, bool) {
	s, ok := ctx.Value(sessionUUIDKey).(string)

	return s, ok && s != ""
}

// WithUserUUID returns a copy of ctx carrying the authenticated user's UUID.
func WithUserUUID(ctx context.Context, userUUID uuid.UUID) context.Context {
	return context.WithValue(ctx, userUUIDKey, userUUID)
}

// UserUUIDFromContext returns the user UUID stored by WithUserUUID.
func UserUUIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	u, ok := ctx.Value(userUUIDKey).(uuid.UUID)

	return u, ok
}

// UnaryClientInterceptor forwards the session from ctx (see WithSessionUUID)
// to the called service as outgoing metadata. Incoming metadata of the current
// request is never propagated automatically in gRPC, so this is what lets a
// downstream service authenticate the same user. Calls made without a session
// (e.g. from a Kafka consumer) go out unchanged.
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if sessionUUID, ok := SessionUUIDFromContext(ctx); ok {
			ctx = metadata.AppendToOutgoingContext(ctx, SessionMetadataKey, sessionUUID)
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
