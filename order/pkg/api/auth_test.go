package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/omaigo88/order/pkg/api"
	"github.com/omaigo88/order/pkg/api/mocks"
	"github.com/omaigo88/shared/pkg/auth"
	authv1 "github.com/omaigo88/shared/pkg/proto/auth/v1"
)

// serve runs one request through AuthMiddleware and reports whether the
// wrapped handler was reached.
func serve(t *testing.T, iam api.IAMClient, authorization string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()

	var reached *http.Request

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = r
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/x", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	rec := httptest.NewRecorder()
	api.AuthMiddleware(iam)(next).ServeHTTP(rec, req)

	return rec, reached
}

func assertErrorBody(t *testing.T, rec *httptest.ResponseRecorder, code int) {
	t.Helper()

	assert.Equal(t, code, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, code, body.Code)
	assert.NotEmpty(t, body.Message)
}

func TestAuthMiddleware_ValidSessionPopulatesContext(t *testing.T) {
	userUUID := uuid.New()

	iam := mocks.NewMockIAMClient(t)
	iam.EXPECT().Whoami(mock.Anything, &authv1.WhoamiRequest{SessionUuid: "session-1"}).
		Return(&authv1.WhoamiResponse{UserUuid: userUUID.String(), Login: "alice"}, nil)

	rec, reached := serve(t, iam, "Bearer session-1")
	require.NotNil(t, reached, "handler must be reached")
	assert.Equal(t, http.StatusNoContent, rec.Code)

	session, ok := auth.SessionUUIDFromContext(reached.Context())
	require.True(t, ok)
	assert.Equal(t, "session-1", session)

	user, ok := auth.UserUUIDFromContext(reached.Context())
	require.True(t, ok)
	assert.Equal(t, userUUID, user)
}

func TestAuthMiddleware_MissingOrMalformedHeader(t *testing.T) {
	for _, header := range []string{"", "session-1", "Basic abc", "Bearer ", "bearer session-1"} {
		t.Run(header, func(t *testing.T) {
			// No IAM expectations: a malformed header never reaches IAM.
			rec, reached := serve(t, mocks.NewMockIAMClient(t), header)
			assert.Nil(t, reached)
			assertErrorBody(t, rec, http.StatusUnauthorized)
		})
	}
}

func TestAuthMiddleware_RejectedSession(t *testing.T) {
	iam := mocks.NewMockIAMClient(t)
	iam.EXPECT().Whoami(mock.Anything, mock.Anything).Return(nil, status.Error(codes.Unauthenticated, "session not found"))

	rec, reached := serve(t, iam, "Bearer expired")
	assert.Nil(t, reached)
	assertErrorBody(t, rec, http.StatusUnauthorized)
}

func TestAuthMiddleware_IAMUnavailable(t *testing.T) {
	iam := mocks.NewMockIAMClient(t)
	iam.EXPECT().Whoami(mock.Anything, mock.Anything).Return(nil, status.Error(codes.Unavailable, "connection refused"))

	rec, reached := serve(t, iam, "Bearer session-1")
	assert.Nil(t, reached)
	assertErrorBody(t, rec, http.StatusInternalServerError)
}

func TestAuthMiddleware_InvalidUserUUIDFromIAM(t *testing.T) {
	iam := mocks.NewMockIAMClient(t)
	iam.EXPECT().Whoami(mock.Anything, mock.Anything).Return(&authv1.WhoamiResponse{UserUuid: "garbage"}, nil)

	rec, reached := serve(t, iam, "Bearer session-1")
	assert.Nil(t, reached)
	assertErrorBody(t, rec, http.StatusInternalServerError)
}
