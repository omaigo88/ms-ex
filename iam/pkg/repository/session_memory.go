package repository

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type memorySession struct {
	session   Session
	expiresAt time.Time
}

// SessionMemoryRepository is an in-memory session store that honours TTLs
// like SessionRedisRepository.
type SessionMemoryRepository struct {
	mu       sync.Mutex
	sessions map[uuid.UUID]memorySession
	now      func() time.Time
}

// NewSessionMemoryRepository creates an empty SessionMemoryRepository.
func NewSessionMemoryRepository() *SessionMemoryRepository {
	return &SessionMemoryRepository{sessions: make(map[uuid.UUID]memorySession), now: time.Now}
}

// Create stores the session until ttl elapses.
func (r *SessionMemoryRepository) Create(_ context.Context, session Session, ttl time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sessions[session.UUID] = memorySession{session: session, expiresAt: r.now().Add(ttl)}

	return nil
}

// Get returns a live session; unknown or expired yields ErrSessionNotFound.
func (r *SessionMemoryRepository) Get(_ context.Context, id uuid.UUID) (Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.sessions[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}

	if !r.now().Before(s.expiresAt) {
		delete(r.sessions, id)

		return Session{}, ErrSessionNotFound
	}

	return s.session, nil
}

// Delete removes a session. Deleting an unknown session is not an error.
func (r *SessionMemoryRepository) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.sessions, id)

	return nil
}
