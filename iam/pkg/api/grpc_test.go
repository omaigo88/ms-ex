package api_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/omaigo88/iam/pkg/api"
	"github.com/omaigo88/iam/pkg/api/mocks"
	"github.com/omaigo88/iam/pkg/repository"
	iamService "github.com/omaigo88/iam/pkg/service"
	authv1 "github.com/omaigo88/shared/pkg/proto/auth/v1"
	userv1 "github.com/omaigo88/shared/pkg/proto/user/v1"
)

func TestAuthServer_Register(t *testing.T) {
	id := uuid.New()

	svc := mocks.NewMockService(t)
	svc.EXPECT().Register(mock.Anything, "alice", "pw").Return(id, nil)

	resp, err := api.NewAuthServer(svc).Register(context.Background(), &authv1.RegisterRequest{Login: "alice", Password: "pw"})
	require.NoError(t, err)
	assert.Equal(t, id.String(), resp.GetUserUuid())
}

func TestAuthServer_Login(t *testing.T) {
	id := uuid.New()

	svc := mocks.NewMockService(t)
	svc.EXPECT().Login(mock.Anything, "alice", "pw").Return(id, nil)

	resp, err := api.NewAuthServer(svc).Login(context.Background(), &authv1.LoginRequest{Login: "alice", Password: "pw"})
	require.NoError(t, err)
	assert.Equal(t, id.String(), resp.GetSessionUuid())
}

func TestAuthServer_Whoami(t *testing.T) {
	session := repository.Session{UUID: uuid.New(), UserUUID: uuid.New(), Login: "alice"}

	svc := mocks.NewMockService(t)
	svc.EXPECT().Whoami(mock.Anything, session.UUID.String()).Return(session, nil)

	resp, err := api.NewAuthServer(svc).Whoami(context.Background(), &authv1.WhoamiRequest{SessionUuid: session.UUID.String()})
	require.NoError(t, err)
	assert.Equal(t, session.UserUUID.String(), resp.GetUserUuid())
	assert.Equal(t, "alice", resp.GetLogin())
}

func TestAuthServer_Logout(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().Logout(mock.Anything, "s").Return(nil)

	_, err := api.NewAuthServer(svc).Logout(context.Background(), &authv1.LogoutRequest{SessionUuid: "s"})
	require.NoError(t, err)
}

func TestUserServer_GetUser(t *testing.T) {
	user := repository.User{UUID: uuid.New(), Login: "alice", CreatedAt: time.Now()}

	svc := mocks.NewMockService(t)
	svc.EXPECT().GetUser(mock.Anything, user.UUID.String()).Return(user, nil)

	resp, err := api.NewUserServer(svc).GetUser(context.Background(), &userv1.GetUserRequest{UserUuid: user.UUID.String()})
	require.NoError(t, err)
	assert.Equal(t, user.UUID.String(), resp.GetUser().GetUuid())
	assert.Equal(t, "alice", resp.GetUser().GetLogin())
	assert.True(t, user.CreatedAt.Equal(resp.GetUser().GetCreatedAt().AsTime()))
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		err  error
		want codes.Code
	}{
		{iamService.ErrEmptyLogin, codes.InvalidArgument},
		{iamService.ErrEmptyPassword, codes.InvalidArgument},
		{iamService.ErrPasswordTooLong, codes.InvalidArgument},
		{iamService.ErrInvalidUUID, codes.InvalidArgument},
		{iamService.ErrUserAlreadyExists, codes.AlreadyExists},
		{iamService.ErrInvalidCredentials, codes.Unauthenticated},
		{iamService.ErrSessionNotFound, codes.Unauthenticated},
		{iamService.ErrUserNotFound, codes.NotFound},
		{errors.New("boom"), codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			svc := mocks.NewMockService(t)
			svc.EXPECT().Whoami(mock.Anything, mock.Anything).Return(repository.Session{}, tt.err)

			_, err := api.NewAuthServer(svc).Whoami(context.Background(), &authv1.WhoamiRequest{SessionUuid: "s"})
			assert.Equal(t, tt.want, status.Code(err))
		})
	}
}
