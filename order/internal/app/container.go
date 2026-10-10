// Package app wires OrderService's dependency graph: repository,
// transaction manager, gRPC clients, business logic, and the HTTP
// transport adapter.
package app

import (
	"context"
	"fmt"
	"os"
	"time"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/omaigo88/order/pkg/api"
	"github.com/omaigo88/order/pkg/repository"
	"github.com/omaigo88/order/pkg/service"
	orderv1 "github.com/omaigo88/shared/pkg/openapi/order/v1"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
	paymentv1 "github.com/omaigo88/shared/pkg/proto/payment/v1"
)

const (
	defaultInventoryServiceAddress = "localhost:50051"
	defaultPaymentServiceAddress   = "localhost:50052"
)

var grpcKeepaliveParams = grpc.WithKeepaliveParams(keepalive.ClientParameters{
	Time:                30 * time.Second,
	Timeout:             10 * time.Second,
	PermitWithoutStream: true,
})

// getEnv returns the environment variable named by key, or fallback if unset/empty.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

// Container holds OrderService's constructed dependencies.
type Container struct {
	pool          *pgxpool.Pool
	inventoryConn *grpc.ClientConn
	paymentConn   *grpc.ClientConn

	// Server is the ogen-generated OpenAPI HTTP handler, ready to pass to an http.Server.
	Server *orderv1.Server
}

// New builds a Container: connects to Postgres (DB_URI) and to
// InventoryService/PaymentService (INVENTORY_SERVICE_ADDRESS/
// PAYMENT_SERVICE_ADDRESS, defaulting to localhost).
func New(ctx context.Context) (*Container, error) {
	pool, err := pgxpool.New(ctx, os.Getenv("DB_URI"))
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	trManager := manager.Must(trmpgx.NewDefaultFactory(pool))

	inventoryConn, err := grpc.NewClient(
		getEnv("INVENTORY_SERVICE_ADDRESS", defaultInventoryServiceAddress),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpcKeepaliveParams,
	)
	if err != nil {
		pool.Close()

		return nil, fmt.Errorf("connect to InventoryService: %w", err)
	}

	paymentConn, err := grpc.NewClient(
		getEnv("PAYMENT_SERVICE_ADDRESS", defaultPaymentServiceAddress),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpcKeepaliveParams,
	)
	if err != nil {
		pool.Close()
		_ = inventoryConn.Close()

		return nil, fmt.Errorf("connect to PaymentService: %w", err)
	}

	repo := repository.NewPostgresRepository(pool)
	svc := service.NewService(
		inventoryv1.NewInventoryServiceClient(inventoryConn),
		paymentv1.NewPaymentServiceClient(paymentConn),
		repo,
		trManager,
	)
	handler := api.NewHandler(svc)

	server, err := api.SetupServer(handler)
	if err != nil {
		pool.Close()
		_ = inventoryConn.Close()
		_ = paymentConn.Close()

		return nil, fmt.Errorf("setup OpenAPI server: %w", err)
	}

	return &Container{
		pool:          pool,
		inventoryConn: inventoryConn,
		paymentConn:   paymentConn,
		Server:        server,
	}, nil
}

// Close releases the Container's resources.
func (c *Container) Close() {
	c.pool.Close()
	_ = c.inventoryConn.Close()
	_ = c.paymentConn.Close()
}
