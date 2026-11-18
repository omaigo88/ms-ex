package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Session hash fields.
const (
	fieldUserUUID  = "user_uuid"
	fieldLogin     = "login"
	fieldCreatedAt = "created_at"
)

// SessionRedisRepository stores each session as a Redis hash
// "session:{uuid}" that expires after the session TTL.
type SessionRedisRepository struct {
	client redis.UniversalClient
}

// NewSessionRedisRepository creates a SessionRedisRepository using client.
func NewSessionRedisRepository(client redis.UniversalClient) *SessionRedisRepository {
	return &SessionRedisRepository{client: client}
}

func sessionKey(id uuid.UUID) string {
	return "session:" + id.String()
}

// Create stores the session (HSET) and sets its TTL (EXPIRE) in one
// MULTI/EXEC, so a session never exists without an expiry.
func (r *SessionRedisRepository) Create(ctx context.Context, session Session, ttl time.Duration) error {
	key := sessionKey(session.UUID)

	_, err := r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, key, map[string]any{
			fieldUserUUID:  session.UserUUID.String(),
			fieldLogin:     session.Login,
			fieldCreatedAt: session.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
		pipe.Expire(ctx, key, ttl)

		return nil
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	return nil
}

// Get returns a session (HGETALL). Redis drops expired keys, so an expired
// session is indistinguishable from an unknown one: both yield ErrSessionNotFound.
func (r *SessionRedisRepository) Get(ctx context.Context, id uuid.UUID) (Session, error) {
	fields, err := r.client.HGetAll(ctx, sessionKey(id)).Result()
	if err != nil {
		return Session{}, fmt.Errorf("get session: %w", err)
	}

	if len(fields) == 0 {
		return Session{}, ErrSessionNotFound
	}

	userUUID, err := uuid.Parse(fields[fieldUserUUID])
	if err != nil {
		return Session{}, fmt.Errorf("get session: parse user_uuid: %w", err)
	}

	createdAt, err := time.Parse(time.RFC3339Nano, fields[fieldCreatedAt])
	if err != nil {
		return Session{}, fmt.Errorf("get session: parse created_at: %w", err)
	}

	return Session{
		UUID:      id,
		UserUUID:  userUUID,
		Login:     fields[fieldLogin],
		CreatedAt: createdAt,
	}, nil
}

// Delete removes a session (DEL). Deleting an unknown session is not an error.
func (r *SessionRedisRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.client.Del(ctx, sessionKey(id)).Err(); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}
