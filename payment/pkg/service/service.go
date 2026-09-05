// Package service contains the PaymentService business logic.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	paymentv1 "github.com/omaigo88/shared/pkg/proto/payment/v1"
)

var (
	// ErrEmptyOrderUUID is returned when order_uuid is empty.
	ErrEmptyOrderUUID = errors.New("order_uuid must not be empty")
	// ErrInvalidOrderUUID is returned when order_uuid cannot be parsed.
	ErrInvalidOrderUUID = errors.New("invalid order_uuid format")
	// ErrUnspecifiedMethod is returned when payment_method is unspecified.
	ErrUnspecifiedMethod = errors.New("payment_method must not be unspecified")
)

// Service implements the PaymentService business logic.
type Service struct{}

// NewService creates a new Service.
func NewService() *Service {
	return &Service{}
}

// PayOrder validates the payment request and returns a new transaction UUID.
func (s *Service) PayOrder(_ context.Context, orderUUID string, method paymentv1.PaymentMethod) (string, error) {
	if orderUUID == "" {
		return "", ErrEmptyOrderUUID
	}

	if method == paymentv1.PaymentMethod_PAYMENT_METHOD_UNSPECIFIED {
		return "", ErrUnspecifiedMethod
	}

	if _, err := uuid.Parse(orderUUID); err != nil {
		return "", fmt.Errorf("%w: %s", ErrInvalidOrderUUID, orderUUID)
	}

	transactionUUID := uuid.New()

	slog.Info(
		"payment succeeded",
		"order_uuid", orderUUID,
		"transaction_uuid", transactionUUID.String(),
	)

	return transactionUUID.String(), nil
}
