package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/omaigo88/order/pkg/repository"
)

// OrderPaidEvent is emitted once an order has been paid.
type OrderPaidEvent struct {
	OrderUUID       uuid.UUID
	UserUUID        uuid.UUID
	PaymentMethod   repository.PaymentMethod
	TransactionUUID uuid.UUID
}

// OrderPaidPublisher delivers OrderPaid events to the rest of the system.
type OrderPaidPublisher interface {
	PublishOrderPaid(ctx context.Context, event OrderPaidEvent) error
}

// noopOrderPaidPublisher discards events. Used where no message broker is
// wired up (the self-contained API test suite).
type noopOrderPaidPublisher struct{}

// NewNoopOrderPaidPublisher creates an OrderPaidPublisher that discards events.
func NewNoopOrderPaidPublisher() OrderPaidPublisher {
	return noopOrderPaidPublisher{}
}

func (noopOrderPaidPublisher) PublishOrderPaid(context.Context, OrderPaidEvent) error {
	return nil
}
