package kafka_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	assemblykafka "github.com/omaigo88/assembly/pkg/kafka"
	"github.com/omaigo88/assembly/pkg/kafka/mocks"
	"github.com/omaigo88/assembly/pkg/service"
	sharedkafka "github.com/omaigo88/shared/pkg/kafka"
	eventsv1 "github.com/omaigo88/shared/pkg/proto/events/v1"
)

func orderPaidMessage(t *testing.T, orderUUID, userUUID string) *sarama.ConsumerMessage {
	t.Helper()

	payload, err := proto.Marshal(&eventsv1.OrderPaid{OrderUuid: orderUUID, UserUuid: userUUID})
	require.NoError(t, err)

	return &sarama.ConsumerMessage{Topic: sharedkafka.TopicOrderPaid, Value: payload}
}

func TestOrderPaidHandler_AssemblesAndPublishes(t *testing.T) {
	order := service.OrderPaid{OrderUUID: uuid.New(), UserUUID: uuid.New()}

	assembler := mocks.NewMockAssembler(t)
	assembler.EXPECT().Assemble(context.Background(), order).Return(service.ShipAssembled{
		OrderUUID: order.OrderUUID, UserUUID: order.UserUUID, BuildTime: 3 * time.Second,
	}, nil)

	sender := mocks.NewMockSender(t)
	sender.EXPECT().
		Send(context.Background(), sharedkafka.TopicShipAssembled, order.OrderUUID.String(), mock.Anything).
		RunAndReturn(func(_ context.Context, _, _ string, msg proto.Message) error {
			got, ok := msg.(*eventsv1.ShipAssembled)
			require.True(t, ok)
			assert.Equal(t, order.OrderUUID.String(), got.GetOrderUuid())
			assert.Equal(t, order.UserUUID.String(), got.GetUserUuid(), "user_uuid must be carried through")
			assert.Equal(t, int64(3), got.GetBuildTimeSec())
			assert.NotEmpty(t, got.GetEventUuid())

			return nil
		})

	handle := assemblykafka.OrderPaidHandler(assembler, sender)
	require.NoError(t, handle(context.Background(), orderPaidMessage(t, order.OrderUUID.String(), order.UserUUID.String())))
}

func TestOrderPaidHandler_AssemblyErrorIsRetried(t *testing.T) {
	assembler := mocks.NewMockAssembler(t)
	assembler.EXPECT().Assemble(context.Background(), mock.Anything).Return(service.ShipAssembled{}, errors.New("boom"))

	handle := assemblykafka.OrderPaidHandler(assembler, mocks.NewMockSender(t))
	require.Error(t, handle(context.Background(), orderPaidMessage(t, uuid.NewString(), uuid.NewString())))
}

func TestOrderPaidHandler_PublishErrorIsRetried(t *testing.T) {
	assembler := mocks.NewMockAssembler(t)
	assembler.EXPECT().Assemble(context.Background(), mock.Anything).Return(service.ShipAssembled{OrderUUID: uuid.New()}, nil)

	sender := mocks.NewMockSender(t)
	sender.EXPECT().Send(context.Background(), mock.Anything, mock.Anything, mock.Anything).Return(errors.New("broker down"))

	handle := assemblykafka.OrderPaidHandler(assembler, sender)
	require.Error(t, handle(context.Background(), orderPaidMessage(t, uuid.NewString(), uuid.NewString())))
}

func TestOrderPaidHandler_MalformedMessagesAreDropped(t *testing.T) {
	handle := assemblykafka.OrderPaidHandler(mocks.NewMockAssembler(t), mocks.NewMockSender(t))

	require.NoError(t, handle(context.Background(), orderPaidMessage(t, "not-a-uuid", uuid.NewString())))
	require.NoError(t, handle(context.Background(), orderPaidMessage(t, uuid.NewString(), "not-a-uuid")))
}
