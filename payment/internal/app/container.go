// Package app wires PaymentService's dependency graph: telemetry (logs,
// traces), business logic and the gRPC transport adapter. PaymentService is
// otherwise stateless, so telemetry is the only thing to close.
package app

import (
	"context"

	"github.com/omaigo88/payment/pkg/api"
	"github.com/omaigo88/payment/pkg/service"
	"github.com/omaigo88/shared/pkg/logger"
	"github.com/omaigo88/shared/pkg/telemetry"
	"github.com/omaigo88/shared/pkg/tracing"
)

const serviceName = "payment-service"

// Container holds PaymentService's constructed dependencies.
type Container struct {
	telemetry []telemetry.ShutdownFunc

	// Server implements paymentv1.PaymentServiceServer, ready to register
	// with a grpc.Server.
	Server *api.Server
}

// New builds a Container.
func New(ctx context.Context) (*Container, error) {
	shutdowns, err := telemetry.Init(ctx, serviceName, logger.Init, tracing.InitTracer)
	if err != nil {
		return nil, err
	}

	svc := service.NewService()
	server := api.NewServer(svc)

	return &Container{telemetry: shutdowns, Server: server}, nil
}

// Close flushes telemetry.
func (c *Container) Close() {
	telemetry.Flush(c.telemetry...)
}
