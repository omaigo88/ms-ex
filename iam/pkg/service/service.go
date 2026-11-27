// Package service contains the IAMService business logic: registration,
// login with bcrypt-hashed passwords, and session lookup/termination.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/omaigo88/iam/pkg/repository"
)

// maxPasswordBytes is bcrypt's input limit; longer passwords are rejected
// rather than silently truncated.
const maxPasswordBytes = 72

var (
	// ErrEmptyLogin is returned when the login is empty.
	ErrEmptyLogin = errors.New("login is required")
	// ErrEmptyPassword is returned when the password is empty.
	ErrEmptyPassword = errors.New("password is required")
	// ErrPasswordTooLong is returned when the password exceeds bcrypt's 72-byte limit.
	ErrPasswordTooLong = errors.New("password must be at most 72 bytes")
	// ErrUserAlreadyExists is returned when the login is already taken.
	ErrUserAlreadyExists = errors.New("user already exists")
	// ErrInvalidCredentials is returned when the login is unknown or the password is wrong.
	ErrInvalidCredentials = errors.New("invalid login or password")
	// ErrSessionNotFound is returned when the session is unknown, expired, or malformed.
	ErrSessionNotFound = errors.New("session not found")
	// ErrInvalidUUID is returned when a UUID argument can't be parsed.
	ErrInvalidUUID = errors.New("invalid uuid")
	// ErrUserNotFound is returned when the user does not exist.
	ErrUserNotFound = errors.New("user not found")
)

// UserRepository is the user storage required by Service.
type UserRepository interface {
	Create(ctx context.Context, user repository.User) error
	GetByLogin(ctx context.Context, login string) (repository.User, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (repository.User, error)
}

// SessionRepository is the session storage required by Service.
type SessionRepository interface {
	Create(ctx context.Context, session repository.Session, ttl time.Duration) error
	Get(ctx context.Context, id uuid.UUID) (repository.Session, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// dummyHash is compared against when the login is unknown, so a failed login
// takes the same bcrypt time whether or not the login exists.
var dummyHash = sync.OnceValue(func() []byte {
	h, _ := bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

	return h
})

// Service implements IAMService's use cases.
type Service struct {
	users      UserRepository
	sessions   SessionRepository
	sessionTTL time.Duration
	now        func() time.Time
}

// NewService creates a Service whose sessions live for sessionTTL.
func NewService(users UserRepository, sessions SessionRepository, sessionTTL time.Duration) *Service {
	return &Service{users: users, sessions: sessions, sessionTTL: sessionTTL, now: time.Now}
}

func validateCredentials(login, password string) error {
	switch {
	case strings.TrimSpace(login) == "":
		return ErrEmptyLogin
	case password == "":
		return ErrEmptyPassword
	case len(password) > maxPasswordBytes:
		return ErrPasswordTooLong
	default:
		return nil
	}
}

// Register creates a user and returns its UUID.
func (s *Service) Register(ctx context.Context, login, password string) (uuid.UUID, error) {
	if err := validateCredentials(login, password); err != nil {
		return uuid.Nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return uuid.Nil, fmt.Errorf("hash password: %w", err)
	}

	now := s.now()
	user := repository.User{
		UUID:         uuid.New(),
		Login:        login,
		PasswordHash: string(hash),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.users.Create(ctx, user); err != nil {
		if errors.Is(err, repository.ErrUserAlreadyExists) {
			return uuid.Nil, ErrUserAlreadyExists
		}

		return uuid.Nil, err
	}

	slog.InfoContext(ctx, "user registered", "user_uuid", user.UUID, "login", user.Login)

	return user.UUID, nil
}

// Login checks the credentials and opens a session, returning its UUID.
func (s *Service) Login(ctx context.Context, login, password string) (uuid.UUID, error) {
	if login == "" || password == "" {
		return uuid.Nil, ErrInvalidCredentials
	}

	user, err := s.users.GetByLogin(ctx, login)
	if errors.Is(err, repository.ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))

		return uuid.Nil, ErrInvalidCredentials
	}

	if err != nil {
		return uuid.Nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return uuid.Nil, ErrInvalidCredentials
	}

	session := repository.Session{
		UUID:      uuid.New(),
		UserUUID:  user.UUID,
		Login:     user.Login,
		CreatedAt: s.now(),
	}

	if err := s.sessions.Create(ctx, session, s.sessionTTL); err != nil {
		return uuid.Nil, err
	}

	// Never log the session UUID itself: it is the bearer credential.
	slog.InfoContext(ctx, "user logged in", "user_uuid", user.UUID)

	return session.UUID, nil
}

// Whoami returns the live session identified by sessionUUID. Not cached:
// every call reads the session store, so a logout takes effect immediately.
func (s *Service) Whoami(ctx context.Context, sessionUUID string) (repository.Session, error) {
	id, err := uuid.Parse(sessionUUID)
	if err != nil {
		return repository.Session{}, ErrSessionNotFound
	}

	session, err := s.sessions.Get(ctx, id)
	if errors.Is(err, repository.ErrSessionNotFound) {
		return repository.Session{}, ErrSessionNotFound
	}

	if err != nil {
		return repository.Session{}, err
	}

	return session, nil
}

// Logout closes the session. Closing an unknown session is a no-op.
func (s *Service) Logout(ctx context.Context, sessionUUID string) error {
	id, err := uuid.Parse(sessionUUID)
	if err != nil {
		return ErrInvalidUUID
	}

	if err := s.sessions.Delete(ctx, id); err != nil {
		return err
	}

	slog.InfoContext(ctx, "session closed")

	return nil
}

// GetUser returns a user by UUID.
func (s *Service) GetUser(ctx context.Context, userUUID string) (repository.User, error) {
	id, err := uuid.Parse(userUUID)
	if err != nil {
		return repository.User{}, ErrInvalidUUID
	}

	user, err := s.users.GetByUUID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.User{}, ErrUserNotFound
	}

	if err != nil {
		return repository.User{}, err
	}

	return user, nil
}
