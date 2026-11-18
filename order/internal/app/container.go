// Package app wires OrderService's dependency graph: repository,
// transaction manager, gRPC clients, business logic, and the HTTP
// transport adapter behind the IAM authentication middleware.
package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/omaigo88/order/pkg/api"
	orderkafka "github.com/omaigo88/order/pkg/kafka"
	"github.com/omaigo88/order/pkg/repository"
	"github.com/omaigo88/order/pkg/service"
	"github.com/omaigo88/shared/pkg/auth"
	sharedkafka "github.com/omaigo88/shared/pkg/kafka"
	authv1 "github.com/omaigo88/shared/pkg/proto/auth/v1"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
	paymentv1 "github.com/omaigo88/shared/pkg/proto/payment/v1"
)

const (
	defaultInventoryServiceAddress = "localhost:50051"
	defaultPaymentServiceAddress   = "localhost:50052"
	defaultIAMServiceAddress       = "localhost:50053"
	defaultKafkaBrokers            = "localhost:9094"

	consumerGroupID = "order-service"
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
	// closers release resources in reverse acquisition order.
	closers []func()

	// Handler is the ogen-generated OpenAPI server wrapped in the IAM
	// authentication middleware, ready to pass to an http.Server.
	Handler http.Handler

	consumer *sharedkafka.Consumer
	service  *service.Service
}

// New builds a Container: connects to Postgres (DB_URI), to
// InventoryService/PaymentService/IAMService (INVENTORY_SERVICE_ADDRESS/
// PAYMENT_SERVICE_ADDRESS/IAM_SERVICE_ADDRESS, defaulting to localhost) and
// to Kafka (KAFKA_BROKERS, defaulting to localhost:9094).
func New(ctx context.Context) (_ *Container, err error) {
	c := &Container{}

	defer func() {
		if err != nil {
			c.Close()
		}
	}()

	pool, err := pgxpool.New(ctx, os.Getenv("DB_URI"))
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	c.closers = append(c.closers, pool.Close)

	// Inventory authenticates callers itself, so the session of the HTTP
	// request is forwarded to it as outgoing gRPC metadata.
	inventoryConn, err := c.dial(
		getEnv("INVENTORY_SERVICE_ADDRESS", defaultInventoryServiceAddress),
		grpc.WithUnaryInterceptor(auth.UnaryClientInterceptor()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to InventoryService: %w", err)
	}

	paymentConn, err := c.dial(getEnv("PAYMENT_SERVICE_ADDRESS", defaultPaymentServiceAddress))
	if err != nil {
		return nil, fmt.Errorf("connect to PaymentService: %w", err)
	}

	iamConn, err := c.dial(getEnv("IAM_SERVICE_ADDRESS", defaultIAMServiceAddress))
	if err != nil {
		return nil, fmt.Errorf("connect to IAMService: %w", err)
	}

	brokers := sharedkafka.ParseBrokers(getEnv("KAFKA_BROKERS", defaultKafkaBrokers))

	producer, err := sharedkafka.NewProducer(brokers)
	if err != nil {
		return nil, err
	}

	c.closers = append(c.closers, func() { _ = producer.Close() })

	c.consumer, err = sharedkafka.NewConsumer(brokers, consumerGroupID, []string{sharedkafka.TopicShipAssembled})
	if err != nil {
		return nil, err
	}

	c.closers = append(c.closers, func() { _ = c.consumer.Close() })

	c.service = service.NewService(
		inventoryv1.NewInventoryServiceClient(inventoryConn),
		paymentv1.NewPaymentServiceClient(paymentConn),
		repository.NewPostgresRepository(pool),
		manager.Must(trmpgx.NewDefaultFactory(pool)),
		orderkafka.NewOrderPaidPublisher(producer),
	)

	server, err := api.SetupServer(api.NewHandler(c.service))
	if err != nil {
		return nil, fmt.Errorf("setup OpenAPI server: %w", err)
	}

	c.Handler = api.AuthMiddleware(authv1.NewAuthServiceClient(iamConn))(server)

	return c, nil
}

func (c *Container) dial(addr string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()), grpcKeepaliveParams)

	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, err
	}

	c.closers = append(c.closers, func() { _ = conn.Close() })

	return conn, nil
}

// RunConsumer consumes ShipAssembled events until ctx is cancelled.
func (c *Container) RunConsumer(ctx context.Context) error {
	return c.consumer.Run(ctx, orderkafka.ShipAssembledHandler(c.service))
}

// Close releases the Container's resources.
func (c *Container) Close() {
	for i := len(c.closers) - 1; i >= 0; i-- {
		c.closers[i]()
	}

	c.closers = nil
}
