package repository

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// UserMemoryRepository is an in-memory user store with the same semantics as
// UserPostgresRepository (unique logins).
type UserMemoryRepository struct {
	mu    sync.RWMutex
	users map[uuid.UUID]User
}

// NewUserMemoryRepository creates an empty UserMemoryRepository.
func NewUserMemoryRepository() *UserMemoryRepository {
	return &UserMemoryRepository{users: make(map[uuid.UUID]User)}
}

// Create stores a user. A taken login yields ErrUserAlreadyExists.
func (r *UserMemoryRepository) Create(_ context.Context, user User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, u := range r.users {
		if u.Login == user.Login {
			return ErrUserAlreadyExists
		}
	}

	r.users[user.UUID] = user

	return nil
}

// GetByLogin returns a user by login.
func (r *UserMemoryRepository) GetByLogin(_ context.Context, login string) (User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, u := range r.users {
		if u.Login == login {
			return u, nil
		}
	}

	return User{}, ErrNotFound
}

// GetByUUID returns a user by UUID.
func (r *UserMemoryRepository) GetByUUID(_ context.Context, id uuid.UUID) (User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	u, ok := r.users[id]
	if !ok {
		return User{}, ErrNotFound
	}

	return u, nil
}
