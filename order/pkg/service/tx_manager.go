package service

import "context"

// TrManager runs fn within a database transaction, committing on success and
// rolling back if fn returns an error.
type TrManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

// noopTrManager is a TrManager that just calls fn directly, with no real
// transaction. Used when Repository isn't backed by a real database (e.g.
// the in-memory repository in tests), where transactions have nothing to
// do.
type noopTrManager struct{}

// NewNoopTrManager creates a TrManager with no transactional behavior.
func NewNoopTrManager() TrManager {
	return noopTrManager{}
}

func (noopTrManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
