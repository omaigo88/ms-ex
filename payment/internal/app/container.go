// Package app wires PaymentService's dependency graph: business logic and
// the gRPC transport adapter. PaymentService is stateless, so there's no
// I/O to set up or close.
package app

import (
	"github.com/omaigo88/payment/pkg/api"
	"github.com/omaigo88/payment/pkg/service"
)

// Container holds PaymentService's constructed dependencies.
type Container struct {
	// Server implements paymentv1.PaymentServiceServer, ready to register
	// with a grpc.Server.
	Server *api.Server
}

// New builds a Container.
func New() *Container {
	svc := service.NewService()
	server := api.NewServer(svc)

	return &Container{Server: server}
}
