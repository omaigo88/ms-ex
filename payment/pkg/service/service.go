package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paymentv1 "github.com/omaigo88/shared/pkg/proto/payment/v1"
)

// server implements the gRPC payment service
type server struct {
	paymentv1.UnimplementedPaymentServiceServer
}

// NewServer creates a new instance of the payment server
func NewServer() *server {
	return &server{}
}

// PayOrder handles payment for an order
func (s *server) PayOrder(
	ctx context.Context,
	req *paymentv1.PayOrderRequest,
) (*paymentv1.PayOrderResponse, error) {
	if req.GetOrderUuid() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_uuid must not be empty")
	}

	if req.GetPaymentMethod() == paymentv1.PaymentMethod_PAYMENT_METHOD_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "payment_method must not be unspecified")
	}

	if _, err := uuid.Parse(req.GetOrderUuid()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid order_uuid format: %s", req.GetOrderUuid())
	}

	transactionUUID := uuid.New()

	slog.Info(
		"payment succeeded",
		"order_uuid", req.GetOrderUuid(),
		"transaction_uuid", transactionUUID.String(),
	)

	return &paymentv1.PayOrderResponse{
		TransactionUuid: transactionUUID.String(),
	}, nil
}
