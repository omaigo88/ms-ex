package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// uniqueViolation is the PostgreSQL error code for a unique constraint violation.
const uniqueViolation = "23505"

const selectUserColumns = `
	SELECT uuid, login, password_hash, created_at, updated_at
	FROM users`

// UserPostgresRepository stores users in PostgreSQL.
type UserPostgresRepository struct {
	pool *pgxpool.Pool
}

// NewUserPostgresRepository creates a UserPostgresRepository backed by pool.
func NewUserPostgresRepository(pool *pgxpool.Pool) *UserPostgresRepository {
	return &UserPostgresRepository{pool: pool}
}

// Create inserts a user. A taken login yields ErrUserAlreadyExists.
func (r *UserPostgresRepository) Create(ctx context.Context, user User) error {
	_, err := r.pool.Exec(
		ctx,
		`INSERT INTO users (uuid, login, password_hash, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)`,
		user.UUID, user.Login, user.PasswordHash, user.CreatedAt, user.UpdatedAt,
	)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return ErrUserAlreadyExists
	}

	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	return nil
}

// GetByLogin returns a user by login.
func (r *UserPostgresRepository) GetByLogin(ctx context.Context, login string) (User, error) {
	return r.get(ctx, selectUserColumns+` WHERE login = $1`, login)
}

// GetByUUID returns a user by UUID.
func (r *UserPostgresRepository) GetByUUID(ctx context.Context, id uuid.UUID) (User, error) {
	return r.get(ctx, selectUserColumns+` WHERE uuid = $1`, id)
}

func (r *UserPostgresRepository) get(ctx context.Context, query string, arg any) (User, error) {
	var user User

	err := r.pool.QueryRow(ctx, query, arg).Scan(
		&user.UUID, &user.Login, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}

	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}

	return user, nil
}
