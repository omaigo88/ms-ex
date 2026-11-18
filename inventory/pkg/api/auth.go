package api

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/omaigo88/shared/pkg/auth"
	authv1 "github.com/omaigo88/shared/pkg/proto/auth/v1"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
)

// IAMClient is the subset of authv1.AuthServiceClient used by AuthInterceptor.
type IAMClient interface {
	Whoami(ctx context.Context, in *authv1.WhoamiRequest, opts ...grpc.CallOption) (*authv1.WhoamiResponse, error)
}

// unauthenticatedMethods don't require a session. CommitParts is called by
// OrderService from its ShipAssembled Kafka consumer, where no user session
// exists (and a 24 h session may have expired before assembly finishes).
var unauthenticatedMethods = map[string]bool{
	inventoryv1.InventoryService_CommitParts_FullMethodName: true,
}

// AuthInterceptor authenticates every unary call (except
// unauthenticatedMethods) with the session UUID from the *incoming* metadata
// key auth.SessionMetadataKey, by calling IAM Whoami on every request. On
// success the session and its user are put on the handler's context.
func AuthInterceptor(iam IAMClient) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if unauthenticatedMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		md, _ := metadata.FromIncomingContext(ctx)

		sessions := md.Get(auth.SessionMetadataKey)
		if len(sessions) == 0 || sessions[0] == "" {
			return nil, status.Error(codes.Unauthenticated, "missing session")
		}

		sessionUUID := sessions[0]

		resp, err := iam.Whoami(ctx, &authv1.WhoamiRequest{SessionUuid: sessionUUID})
		if err != nil {
			if status.Code(err) == codes.Unauthenticated {
				return nil, status.Error(codes.Unauthenticated, "invalid or expired session")
			}

			return nil, status.Errorf(codes.Internal, "authenticate: %v", err)
		}

		userUUID, err := uuid.Parse(resp.GetUserUuid())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "authenticate: invalid user_uuid %q", resp.GetUserUuid())
		}

		return handler(auth.WithUserUUID(auth.WithSessionUUID(ctx, sessionUUID), userUUID), req)
	}
}
