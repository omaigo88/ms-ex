// Package repository implements IAMService storage: users in PostgreSQL and
// sessions in Redis, each with an in-memory twin for the self-contained API
// test suite.
package repository

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrNotFound is returned when a user does not exist.
	ErrNotFound = errors.New("not found")
	// ErrUserAlreadyExists is returned when the login is already taken.
	ErrUserAlreadyExists = errors.New("user already exists")
	// ErrSessionNotFound is returned when a session does not exist or has expired.
	ErrSessionNotFound = errors.New("session not found")
)

// User is a registered user.
type User struct {
	UUID         uuid.UUID
	Login        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Session is an authenticated session. It carries the user's login so
// Whoami can be answered from Redis alone, without a database round trip.
type Session struct {
	UUID      uuid.UUID
	UserUUID  uuid.UUID
	Login     string
	CreatedAt time.Time
}
