// Package app wires InventoryService's dependency graph: repository,
// domain services, business logic, and the gRPC transport adapter.
package app

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/omaigo88/inventory/pkg/api"
	"github.com/omaigo88/inventory/pkg/domain"
	"github.com/omaigo88/inventory/pkg/repository"
	"github.com/omaigo88/inventory/pkg/service"
)

// Container holds InventoryService's constructed dependencies.
type Container struct {
	pool *pgxpool.Pool

	// Server implements inventoryv1.InventoryServiceServer, ready to register
	// with a grpc.Server.
	Server *api.Server
}

// New builds a Container, connecting to Postgres using the DB_URI
// environment variable.
func New(ctx context.Context) (*Container, error) {
	pool, err := pgxpool.New(ctx, os.Getenv("DB_URI"))
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	repo := repository.NewPostgresRepository(pool)
	checker := domain.NewCompatibilityChecker()
	svc := service.NewService(repo, checker)
	server := api.NewServer(svc)

	return &Container{
		pool:   pool,
		Server: server,
	}, nil
}

// Close releases the Container's resources.
func (c *Container) Close() {
	c.pool.Close()
}
