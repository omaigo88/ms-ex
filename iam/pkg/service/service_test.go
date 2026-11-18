package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/omaigo88/iam/pkg/repository"
	"github.com/omaigo88/iam/pkg/service"
	"github.com/omaigo88/iam/pkg/service/mocks"
)

const ttl = 24 * time.Hour

type deps struct {
	users    *mocks.MockUserRepository
	sessions *mocks.MockSessionRepository
	svc      *service.Service
}

func newDeps(t *testing.T) deps {
	t.Helper()

	d := deps{
		users:    mocks.NewMockUserRepository(t),
		sessions: mocks.NewMockSessionRepository(t),
	}
	d.svc = service.NewService(d.users, d.sessions, ttl)

	return d
}

// userWithPassword returns a stored user whose hash matches password
// (MinCost keeps the tests fast; Compare works for any cost).
func userWithPassword(t *testing.T, login, password string) repository.User {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)

	return repository.User{UUID: uuid.New(), Login: login, PasswordHash: string(hash), CreatedAt: time.Now()}
}

func TestService_Register_HashesPassword(t *testing.T) {
	d := newDeps(t)

	var stored repository.User

	d.users.EXPECT().Create(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, u repository.User) error {
		stored = u

		return nil
	})

	id, err := d.svc.Register(context.Background(), "alice", "s3cret")
	require.NoError(t, err)
	assert.Equal(t, stored.UUID, id)
	assert.Equal(t, "alice", stored.Login)
	assert.NotEqual(t, "s3cret", stored.PasswordHash, "password must not be stored in plain text")
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("s3cret")))
}

func TestService_Register_Validation(t *testing.T) {
	tests := []struct {
		name            string
		login, password string
		want            error
	}{
		{"empty login", "", "pw", service.ErrEmptyLogin},
		{"blank login", "   ", "pw", service.ErrEmptyLogin},
		{"empty password", "alice", "", service.ErrEmptyPassword},
		{"password over 72 bytes", "alice", strings.Repeat("x", 73), service.ErrPasswordTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newDeps(t).svc.Register(context.Background(), tt.login, tt.password)
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestService_Register_DuplicateLogin(t *testing.T) {
	d := newDeps(t)
	d.users.EXPECT().Create(mock.Anything, mock.Anything).Return(repository.ErrUserAlreadyExists)

	_, err := d.svc.Register(context.Background(), "alice", "pw")
	require.ErrorIs(t, err, service.ErrUserAlreadyExists)
}

func TestService_Login_CreatesSessionWithTTL(t *testing.T) {
	d := newDeps(t)
	user := userWithPassword(t, "alice", "pw")

	d.users.EXPECT().GetByLogin(mock.Anything, "alice").Return(user, nil)

	var created repository.Session

	d.sessions.EXPECT().Create(mock.Anything, mock.Anything, ttl).
		RunAndReturn(func(_ context.Context, s repository.Session, _ time.Duration) error {
			created = s

			return nil
		})

	sessionUUID, err := d.svc.Login(context.Background(), "alice", "pw")
	require.NoError(t, err)
	assert.Equal(t, created.UUID, sessionUUID)
	assert.Equal(t, user.UUID, created.UserUUID)
	assert.Equal(t, "alice", created.Login)
}

func TestService_Login_InvalidCredentials(t *testing.T) {
	t.Run("wrong password", func(t *testing.T) {
		d := newDeps(t)
		d.users.EXPECT().GetByLogin(mock.Anything, "alice").Return(userWithPassword(t, "alice", "pw"), nil)

		_, err := d.svc.Login(context.Background(), "alice", "wrong")
		require.ErrorIs(t, err, service.ErrInvalidCredentials)
	})

	t.Run("unknown login", func(t *testing.T) {
		d := newDeps(t)
		d.users.EXPECT().GetByLogin(mock.Anything, "bob").Return(repository.User{}, repository.ErrNotFound)

		_, err := d.svc.Login(context.Background(), "bob", "pw")
		require.ErrorIs(t, err, service.ErrInvalidCredentials, "unknown login must look like a wrong password")
	})

	t.Run("empty credentials", func(t *testing.T) {
		_, err := newDeps(t).svc.Login(context.Background(), "", "")
		require.ErrorIs(t, err, service.ErrInvalidCredentials)
	})
}

func TestService_Login_StorageError(t *testing.T) {
	d := newDeps(t)
	d.users.EXPECT().GetByLogin(mock.Anything, "alice").Return(repository.User{}, errors.New("db down"))

	_, err := d.svc.Login(context.Background(), "alice", "pw")
	require.Error(t, err)
	assert.NotErrorIs(t, err, service.ErrInvalidCredentials)
}

func TestService_Whoami(t *testing.T) {
	session := repository.Session{UUID: uuid.New(), UserUUID: uuid.New(), Login: "alice"}

	t.Run("live session", func(t *testing.T) {
		d := newDeps(t)
		d.sessions.EXPECT().Get(mock.Anything, session.UUID).Return(session, nil)

		got, err := d.svc.Whoami(context.Background(), session.UUID.String())
		require.NoError(t, err)
		assert.Equal(t, session, got)
	})

	t.Run("unknown or expired session", func(t *testing.T) {
		d := newDeps(t)
		d.sessions.EXPECT().Get(mock.Anything, session.UUID).Return(repository.Session{}, repository.ErrSessionNotFound)

		_, err := d.svc.Whoami(context.Background(), session.UUID.String())
		require.ErrorIs(t, err, service.ErrSessionNotFound)
	})

	t.Run("malformed session uuid", func(t *testing.T) {
		_, err := newDeps(t).svc.Whoami(context.Background(), "not-a-uuid")
		require.ErrorIs(t, err, service.ErrSessionNotFound)
	})
}

func TestService_Logout(t *testing.T) {
	id := uuid.New()

	d := newDeps(t)
	d.sessions.EXPECT().Delete(mock.Anything, id).Return(nil)
	require.NoError(t, d.svc.Logout(context.Background(), id.String()))

	require.ErrorIs(t, newDeps(t).svc.Logout(context.Background(), "nope"), service.ErrInvalidUUID)
}

func TestService_GetUser(t *testing.T) {
	user := repository.User{UUID: uuid.New(), Login: "alice"}

	d := newDeps(t)
	d.users.EXPECT().GetByUUID(mock.Anything, user.UUID).Return(user, nil)

	got, err := d.svc.GetUser(context.Background(), user.UUID.String())
	require.NoError(t, err)
	assert.Equal(t, user, got)

	d = newDeps(t)
	d.users.EXPECT().GetByUUID(mock.Anything, user.UUID).Return(repository.User{}, repository.ErrNotFound)

	_, err = d.svc.GetUser(context.Background(), user.UUID.String())
	require.ErrorIs(t, err, service.ErrUserNotFound)

	_, err = newDeps(t).svc.GetUser(context.Background(), "nope")
	require.ErrorIs(t, err, service.ErrInvalidUUID)
}
