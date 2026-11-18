package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omaigo88/iam/pkg/repository"
)

func TestUserMemory_CreateAndGet(t *testing.T) {
	repo := repository.NewUserMemoryRepository()
	user := repository.User{UUID: uuid.New(), Login: "alice", PasswordHash: "hash"}

	require.NoError(t, repo.Create(context.Background(), user))
	require.ErrorIs(t, repo.Create(context.Background(), repository.User{UUID: uuid.New(), Login: "alice"}),
		repository.ErrUserAlreadyExists)

	byLogin, err := repo.GetByLogin(context.Background(), "alice")
	require.NoError(t, err)
	assert.Equal(t, user, byLogin)

	byUUID, err := repo.GetByUUID(context.Background(), user.UUID)
	require.NoError(t, err)
	assert.Equal(t, user, byUUID)

	_, err = repo.GetByLogin(context.Background(), "bob")
	require.ErrorIs(t, err, repository.ErrNotFound)

	_, err = repo.GetByUUID(context.Background(), uuid.New())
	require.ErrorIs(t, err, repository.ErrNotFound)
}

func TestSessionMemory_TTLAndDelete(t *testing.T) {
	repo := repository.NewSessionMemoryRepository()
	live := testSession()
	short := testSession()

	require.NoError(t, repo.Create(context.Background(), live, time.Hour))
	require.NoError(t, repo.Create(context.Background(), short, time.Millisecond))

	time.Sleep(5 * time.Millisecond)

	got, err := repo.Get(context.Background(), live.UUID)
	require.NoError(t, err)
	assert.Equal(t, live, got)

	_, err = repo.Get(context.Background(), short.UUID)
	require.ErrorIs(t, err, repository.ErrSessionNotFound, "expired session")

	require.NoError(t, repo.Delete(context.Background(), live.UUID))

	_, err = repo.Get(context.Background(), live.UUID)
	require.ErrorIs(t, err, repository.ErrSessionNotFound)
}
