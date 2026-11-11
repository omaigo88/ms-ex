package service

import "context"

// TrManager runs fn within a database transaction, committing on success and
// rolling back if fn returns an error.
type TrManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

// noopTrManager calls fn directly, with no real transaction. Used with the
// in-memory repository (tests), where there's nothing to commit or roll back.
type noopTrManager struct{}

// NewNoopTrManager creates a TrManager with no transactional behavior.
func NewNoopTrManager() TrManager {
	return noopTrManager{}
}

func (noopTrManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
