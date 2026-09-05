// Package api implements the PaymentService gRPC transport layer.
package api

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paymentService "github.com/omaigo88/payment/pkg/service"
	paymentv1 "github.com/omaigo88/shared/pkg/proto/payment/v1"
)

// Service is the business-logic dependency required by Server.
type Service interface {
	PayOrder(ctx context.Context, orderUUID string, method paymentv1.PaymentMethod) (string, error)
}

// Server implements paymentv1.PaymentServiceServer.
type Server struct {
	paymentv1.UnimplementedPaymentServiceServer
	svc Service
}

// NewServer creates a new Server backed by the given Service.
func NewServer(svc Service) *Server {
	return &Server{svc: svc}
}

// toStatusErr maps a Service sentinel error to a gRPC status error.
func toStatusErr(err error) error {
	switch {
	case errors.Is(err, paymentService.ErrEmptyOrderUUID),
		errors.Is(err, paymentService.ErrInvalidOrderUUID),
		errors.Is(err, paymentService.ErrUnspecifiedMethod):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// PayOrder handles payment for an order.
func (s *Server) PayOrder(ctx context.Context, req *paymentv1.PayOrderRequest) (*paymentv1.PayOrderResponse, error) {
	transactionUUID, err := s.svc.PayOrder(ctx, req.GetOrderUuid(), req.GetPaymentMethod())
	if err != nil {
		return nil, toStatusErr(err)
	}

	return &paymentv1.PayOrderResponse{
		TransactionUuid: transactionUUID,
	}, nil
}
