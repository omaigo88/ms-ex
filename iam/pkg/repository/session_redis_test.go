package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omaigo88/iam/pkg/repository"
)

func newRedisRepo(t *testing.T) (*repository.SessionRedisRepository, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return repository.NewSessionRedisRepository(client), mr
}

func testSession() repository.Session {
	return repository.Session{
		UUID:      uuid.New(),
		UserUUID:  uuid.New(),
		Login:     "alice",
		CreatedAt: time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC),
	}
}

func TestSessionRedis_CreateStoresHashWithTTL(t *testing.T) {
	repo, mr := newRedisRepo(t)
	s := testSession()

	require.NoError(t, repo.Create(context.Background(), s, 24*time.Hour))

	key := "session:" + s.UUID.String()
	assert.Equal(t, s.UserUUID.String(), mr.HGet(key, "user_uuid"))
	assert.Equal(t, "alice", mr.HGet(key, "login"))
	assert.Equal(t, 24*time.Hour, mr.TTL(key))

	got, err := repo.Get(context.Background(), s.UUID)
	require.NoError(t, err)
	assert.Equal(t, s, got)
}

func TestSessionRedis_ExpiredSessionIsNotFound(t *testing.T) {
	repo, mr := newRedisRepo(t)
	s := testSession()

	require.NoError(t, repo.Create(context.Background(), s, time.Minute))
	mr.FastForward(time.Minute)

	_, err := repo.Get(context.Background(), s.UUID)
	require.ErrorIs(t, err, repository.ErrSessionNotFound)
}

func TestSessionRedis_UnknownSessionIsNotFound(t *testing.T) {
	repo, _ := newRedisRepo(t)

	_, err := repo.Get(context.Background(), uuid.New())
	require.ErrorIs(t, err, repository.ErrSessionNotFound)
}

func TestSessionRedis_Delete(t *testing.T) {
	repo, _ := newRedisRepo(t)
	s := testSession()

	require.NoError(t, repo.Create(context.Background(), s, time.Hour))
	require.NoError(t, repo.Delete(context.Background(), s.UUID))

	_, err := repo.Get(context.Background(), s.UUID)
	require.ErrorIs(t, err, repository.ErrSessionNotFound)

	require.NoError(t, repo.Delete(context.Background(), s.UUID), "deleting twice is not an error")
}
