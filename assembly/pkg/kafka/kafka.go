// Package kafka adapts AssemblyService's business logic to Kafka: it consumes
// OrderPaid events and publishes ShipAssembled events.
package kafka

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/omaigo88/assembly/pkg/service"
	sharedkafka "github.com/omaigo88/shared/pkg/kafka"
	eventsv1 "github.com/omaigo88/shared/pkg/proto/events/v1"
)

// Assembler is the subset of service.Assembler used here.
type Assembler interface {
	Assemble(ctx context.Context, order service.OrderPaid) (service.ShipAssembled, error)
}

// Sender is the subset of shared/pkg/kafka.Producer used here.
type Sender interface {
	Send(ctx context.Context, topic, key string, msg proto.Message) error
}

// OrderPaidHandler returns a sharedkafka.Handler that assembles the ship for
// every OrderPaid event and publishes ShipAssembled. A malformed message is
// dropped; an assembly or publish error is returned so the event is retried.
func OrderPaidHandler(assembler Assembler, sender Sender) sharedkafka.Handler {
	return func(ctx context.Context, msg *sarama.ConsumerMessage) error {
		var event eventsv1.OrderPaid
		if err := proto.Unmarshal(msg.Value, &event); err != nil {
			return sharedkafka.DropMalformed(msg, err)
		}

		orderUUID, err := uuid.Parse(event.GetOrderUuid())
		if err != nil {
			return sharedkafka.DropMalformed(msg, err)
		}

		userUUID, err := uuid.Parse(event.GetUserUuid())
		if err != nil {
			return sharedkafka.DropMalformed(msg, err)
		}

		ship, err := assembler.Assemble(ctx, service.OrderPaid{OrderUUID: orderUUID, UserUUID: userUUID})
		if err != nil {
			return fmt.Errorf("assemble order %s: %w", orderUUID, err)
		}

		return sender.Send(ctx, sharedkafka.TopicShipAssembled, ship.OrderUUID.String(), &eventsv1.ShipAssembled{
			EventUuid:    uuid.NewString(),
			OrderUuid:    ship.OrderUUID.String(),
			UserUuid:     ship.UserUUID.String(),
			BuildTimeSec: int64(ship.BuildTime.Seconds()),
		})
	}
}
