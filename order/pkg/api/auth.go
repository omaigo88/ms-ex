package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/omaigo88/shared/pkg/auth"
	authv1 "github.com/omaigo88/shared/pkg/proto/auth/v1"
)

const bearerPrefix = "Bearer "

// IAMClient is the subset of authv1.AuthServiceClient used by AuthMiddleware.
type IAMClient interface {
	Whoami(ctx context.Context, in *authv1.WhoamiRequest, opts ...grpc.CallOption) (*authv1.WhoamiResponse, error)
}

// errorBody mirrors the OpenAPI error schema ({code, message}) so auth
// failures look like every other API error.
type errorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(errorBody{Code: code, Message: message})
}

// AuthMiddleware authenticates every request with the session from
// "Authorization: Bearer <session_uuid>" by calling IAM Whoami (on every
// request — results are deliberately not cached, so a logout takes effect
// immediately). On success the session and its user are put on the request
// context for the handler and for outgoing gRPC calls; otherwise it answers
// 401 without reaching the handler.
func AuthMiddleware(iam IAMClient) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, bearerPrefix) {
				writeError(w, http.StatusUnauthorized, "unauthorized")

				return
			}

			sessionUUID := strings.TrimSpace(strings.TrimPrefix(header, bearerPrefix))
			if sessionUUID == "" {
				writeError(w, http.StatusUnauthorized, "unauthorized")

				return
			}

			resp, err := iam.Whoami(r.Context(), &authv1.WhoamiRequest{SessionUuid: sessionUUID})
			if err != nil {
				if status.Code(err) == codes.Unauthenticated {
					writeError(w, http.StatusUnauthorized, "unauthorized")

					return
				}

				slog.Error("IAM Whoami failed", "error", err)
				writeError(w, http.StatusInternalServerError, "internal server error")

				return
			}

			userUUID, err := uuid.Parse(resp.GetUserUuid())
			if err != nil {
				slog.Error("IAM Whoami returned an invalid user_uuid", "user_uuid", resp.GetUserUuid())
				writeError(w, http.StatusInternalServerError, "internal server error")

				return
			}

			ctx := auth.WithUserUUID(auth.WithSessionUUID(r.Context(), sessionUUID), userUUID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
