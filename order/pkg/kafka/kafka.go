// Package kafka adapts OrderService's business logic to Kafka: it publishes
// OrderPaid events and handles ShipAssembled events.
package kafka

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/omaigo88/order/pkg/service"
	sharedkafka "github.com/omaigo88/shared/pkg/kafka"
	eventsv1 "github.com/omaigo88/shared/pkg/proto/events/v1"
)

// Sender is the subset of shared/pkg/kafka.Producer used here.
type Sender interface {
	Send(ctx context.Context, topic, key string, msg proto.Message) error
}

// OrderPaidPublisher implements service.OrderPaidPublisher on top of Kafka.
type OrderPaidPublisher struct {
	sender Sender
}

// NewOrderPaidPublisher creates an OrderPaidPublisher sending through sender.
func NewOrderPaidPublisher(sender Sender) *OrderPaidPublisher {
	return &OrderPaidPublisher{sender: sender}
}

// PublishOrderPaid sends event to the order.paid topic, keyed by order UUID.
func (p *OrderPaidPublisher) PublishOrderPaid(ctx context.Context, event service.OrderPaidEvent) error {
	return p.sender.Send(ctx, sharedkafka.TopicOrderPaid, event.OrderUUID.String(), &eventsv1.OrderPaid{
		EventUuid:       uuid.NewString(),
		OrderUuid:       event.OrderUUID.String(),
		UserUuid:        event.UserUUID.String(),
		PaymentMethod:   string(event.PaymentMethod),
		TransactionUuid: event.TransactionUUID.String(),
	})
}

// AssemblyMarker is the subset of service.Service used by the ShipAssembled handler.
type AssemblyMarker interface {
	MarkAssembled(ctx context.Context, orderUUID uuid.UUID) error
}

// ShipAssembledHandler returns a sharedkafka.Handler that decodes ShipAssembled
// events and marks the order assembled. A malformed message is dropped (it
// would fail the same way on every redelivery); a business-logic error is
// returned so the message is retried.
func ShipAssembledHandler(svc AssemblyMarker) sharedkafka.Handler {
	return func(ctx context.Context, msg *sarama.ConsumerMessage) error {
		var event eventsv1.ShipAssembled
		if err := proto.Unmarshal(msg.Value, &event); err != nil {
			return sharedkafka.DropMalformed(msg, err)
		}

		orderUUID, err := uuid.Parse(event.GetOrderUuid())
		if err != nil {
			return sharedkafka.DropMalformed(msg, err)
		}

		if err := svc.MarkAssembled(ctx, orderUUID); err != nil {
			return fmt.Errorf("mark order %s assembled: %w", orderUUID, err)
		}

		return nil
	}
}
