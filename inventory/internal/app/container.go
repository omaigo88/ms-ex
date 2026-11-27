// Package app wires InventoryService's dependency graph: telemetry (logs,
// traces), repository,
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
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/omaigo88/inventory/pkg/api"
	"github.com/omaigo88/inventory/pkg/domain"
	"github.com/omaigo88/inventory/pkg/repository"
	"github.com/omaigo88/inventory/pkg/service"
	"github.com/omaigo88/shared/pkg/logger"
	authv1 "github.com/omaigo88/shared/pkg/proto/auth/v1"
	"github.com/omaigo88/shared/pkg/telemetry"
	"github.com/omaigo88/shared/pkg/tracing"
)

const (
	defaultIAMServiceAddress = "localhost:50053"
	serviceName              = "inventory-service"
)

// Container holds InventoryService's constructed dependencies.
type Container struct {
	pool      *pgxpool.Pool
	iamConn   *grpc.ClientConn
	telemetry []telemetry.ShutdownFunc

	// Server implements inventoryv1.InventoryServiceServer, ready to register
	// with a grpc.Server.
	Server *api.Server

	// AuthInterceptor authenticates incoming calls against IAMService.
	AuthInterceptor grpc.UnaryServerInterceptor
}

// New builds a Container, connecting to Postgres (DB_URI) and IAMService
// (IAM_SERVICE_ADDRESS, default localhost:50053).
func New(ctx context.Context) (*Container, error) {
	shutdowns, err := telemetry.Init(ctx, serviceName, logger.Init, tracing.InitTracer)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.New(ctx, os.Getenv("DB_URI"))
	if err != nil {
		telemetry.Flush(shutdowns...)

		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	iamAddress := os.Getenv("IAM_SERVICE_ADDRESS")
	if iamAddress == "" {
		iamAddress = defaultIAMServiceAddress
	}

	// The interceptor's Whoami call is traced as a child of the incoming RPC.
	iamConn, err := grpc.NewClient(
		iamAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		pool.Close()
		telemetry.Flush(shutdowns...)

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
		telemetry:       shutdowns,
		Server:          server,
		AuthInterceptor: api.AuthInterceptor(authv1.NewAuthServiceClient(iamConn)),
	}, nil
}

// Close releases the Container's resources.
func (c *Container) Close() {
	_ = c.iamConn.Close()
	c.pool.Close()
	telemetry.Flush(c.telemetry...)
}
