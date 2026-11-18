// Package api implements the IAMService gRPC transport layer (AuthService and
// UserService).
package api

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/omaigo88/iam/pkg/repository"
	iamService "github.com/omaigo88/iam/pkg/service"
	authv1 "github.com/omaigo88/shared/pkg/proto/auth/v1"
	userv1 "github.com/omaigo88/shared/pkg/proto/user/v1"
)

// Service is the business-logic dependency required by the servers.
type Service interface {
	Register(ctx context.Context, login, password string) (uuid.UUID, error)
	Login(ctx context.Context, login, password string) (uuid.UUID, error)
	Whoami(ctx context.Context, sessionUUID string) (repository.Session, error)
	Logout(ctx context.Context, sessionUUID string) error
	GetUser(ctx context.Context, userUUID string) (repository.User, error)
}

// toStatusErr maps a Service sentinel error to a gRPC status error.
func toStatusErr(err error) error {
	switch {
	case errors.Is(err, iamService.ErrEmptyLogin),
		errors.Is(err, iamService.ErrEmptyPassword),
		errors.Is(err, iamService.ErrPasswordTooLong),
		errors.Is(err, iamService.ErrInvalidUUID):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, iamService.ErrUserAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, iamService.ErrInvalidCredentials),
		errors.Is(err, iamService.ErrSessionNotFound):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, iamService.ErrUserNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// AuthServer implements authv1.AuthServiceServer.
type AuthServer struct {
	authv1.UnimplementedAuthServiceServer
	svc Service
}

// NewAuthServer creates an AuthServer backed by svc.
func NewAuthServer(svc Service) *AuthServer {
	return &AuthServer{svc: svc}
}

// Register handles AuthService.Register.
func (s *AuthServer) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	userUUID, err := s.svc.Register(ctx, req.GetLogin(), req.GetPassword())
	if err != nil {
		return nil, toStatusErr(err)
	}

	return &authv1.RegisterResponse{UserUuid: userUUID.String()}, nil
}

// Login handles AuthService.Login.
func (s *AuthServer) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	sessionUUID, err := s.svc.Login(ctx, req.GetLogin(), req.GetPassword())
	if err != nil {
		return nil, toStatusErr(err)
	}

	return &authv1.LoginResponse{SessionUuid: sessionUUID.String()}, nil
}

// Whoami handles AuthService.Whoami.
func (s *AuthServer) Whoami(ctx context.Context, req *authv1.WhoamiRequest) (*authv1.WhoamiResponse, error) {
	session, err := s.svc.Whoami(ctx, req.GetSessionUuid())
	if err != nil {
		return nil, toStatusErr(err)
	}

	return &authv1.WhoamiResponse{UserUuid: session.UserUUID.String(), Login: session.Login}, nil
}

// Logout handles AuthService.Logout.
func (s *AuthServer) Logout(ctx context.Context, req *authv1.LogoutRequest) (*authv1.LogoutResponse, error) {
	if err := s.svc.Logout(ctx, req.GetSessionUuid()); err != nil {
		return nil, toStatusErr(err)
	}

	return &authv1.LogoutResponse{}, nil
}

// UserServer implements userv1.UserServiceServer.
type UserServer struct {
	userv1.UnimplementedUserServiceServer
	svc Service
}

// NewUserServer creates a UserServer backed by svc.
func NewUserServer(svc Service) *UserServer {
	return &UserServer{svc: svc}
}

// GetUser handles UserService.GetUser.
func (s *UserServer) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	user, err := s.svc.GetUser(ctx, req.GetUserUuid())
	if err != nil {
		return nil, toStatusErr(err)
	}

	return &userv1.GetUserResponse{User: &userv1.User{
		Uuid:      user.UUID.String(),
		Login:     user.Login,
		CreatedAt: timestamppb.New(user.CreatedAt),
	}}, nil
}
