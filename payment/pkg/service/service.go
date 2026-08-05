package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paymentv1 "github.com/omaigo88/shared/pkg/proto/payment/v1"
)

// server реализует gRPC сервис оплаты
type server struct {
	paymentv1.UnimplementedPaymentServiceServer
}

// NewServer создаёт новый экземпляр сервера оплаты
func NewServer() *server {
	return &server{}
}

// PayOrder обрабатывает оплату заказа
func (s *server) PayOrder(
	ctx context.Context,
	req *paymentv1.PayOrderRequest,
) (*paymentv1.PayOrderResponse, error) {
	if req.GetOrderUuid() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_uuid не может быть пустым")
	}

	if req.GetPaymentMethod() == paymentv1.PaymentMethod_PAYMENT_METHOD_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "payment_method не может быть unspecified")
	}

	if _, err := uuid.Parse(req.GetOrderUuid()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "неверный формат order_uuid: %s", req.GetOrderUuid())
	}

	transactionUUID := uuid.New()

	slog.Info("оплата прошла успешно",
		"order_uuid", req.GetOrderUuid(),
		"transaction_uuid", transactionUUID.String(),
	)

	return &paymentv1.PayOrderResponse{
		TransactionUuid: transactionUUID.String(),
	}, nil
}
