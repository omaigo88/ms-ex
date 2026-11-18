// Package app wires InventoryService's dependency graph: repository,
// domain services, business logic, the gRPC transport adapter, and the IAM
// authentication interceptor.
package app

import (
	"context"
	"fmt"
	"os"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/omaigo88/inventory/pkg/api"
	"github.com/omaigo88/inventory/pkg/domain"
	"github.com/omaigo88/inventory/pkg/repository"
	"github.com/omaigo88/inventory/pkg/service"
	authv1 "github.com/omaigo88/shared/pkg/proto/auth/v1"
)

const defaultIAMServiceAddress = "localhost:50053"

// Container holds InventoryService's constructed dependencies.
type Container struct {
	pool    *pgxpool.Pool
	iamConn *grpc.ClientConn

	// Server implements inventoryv1.InventoryServiceServer, ready to register
	// with a grpc.Server.
	Server *api.Server

	// AuthInterceptor authenticates incoming calls against IAMService.
	AuthInterceptor grpc.UnaryServerInterceptor
}

// New builds a Container, connecting to Postgres (DB_URI) and IAMService
// (IAM_SERVICE_ADDRESS, default localhost:50053).
func New(ctx context.Context) (*Container, error) {
	pool, err := pgxpool.New(ctx, os.Getenv("DB_URI"))
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	iamAddress := os.Getenv("IAM_SERVICE_ADDRESS")
	if iamAddress == "" {
		iamAddress = defaultIAMServiceAddress
	}

	iamConn, err := grpc.NewClient(iamAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		pool.Close()

		return nil, fmt.Errorf("connect to IAMService: %w", err)
	}

	repo := repository.NewPostgresRepository(pool)
	checker := domain.NewCompatibilityChecker()
	trManager := manager.Must(trmpgx.NewDefaultFactory(pool))
	svc := service.NewService(repo, checker, trManager)
	server := api.NewServer(svc)

	return &Container{
		pool:            pool,
		iamConn:         iamConn,
		Server:          server,
		AuthInterceptor: api.AuthInterceptor(authv1.NewAuthServiceClient(iamConn)),
	}, nil
}

// Close releases the Container's resources.
func (c *Container) Close() {
	_ = c.iamConn.Close()
	c.pool.Close()
}
