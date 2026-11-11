package kafka_test

import (
	"context"
	"errors"
	"testing"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	orderkafka "github.com/omaigo88/order/pkg/kafka"
	"github.com/omaigo88/order/pkg/kafka/mocks"
	"github.com/omaigo88/order/pkg/repository"
	"github.com/omaigo88/order/pkg/service"
	sharedkafka "github.com/omaigo88/shared/pkg/kafka"
	eventsv1 "github.com/omaigo88/shared/pkg/proto/events/v1"
)

func TestOrderPaidPublisher_SendsEventKeyedByOrder(t *testing.T) {
	event := service.OrderPaidEvent{
		OrderUUID:       uuid.New(),
		UserUUID:        uuid.New(),
		PaymentMethod:   repository.PaymentMethodSBP,
		TransactionUUID: uuid.New(),
	}

	sender := mocks.NewMockSender(t)
	sender.EXPECT().
		Send(context.Background(), sharedkafka.TopicOrderPaid, event.OrderUUID.String(), mock.Anything).
		RunAndReturn(func(_ context.Context, _, _ string, msg proto.Message) error {
			got, ok := msg.(*eventsv1.OrderPaid)
			require.True(t, ok)
			assert.Equal(t, event.OrderUUID.String(), got.GetOrderUuid())
			assert.Equal(t, event.UserUUID.String(), got.GetUserUuid())
			assert.Equal(t, "SBP", got.GetPaymentMethod())
			assert.Equal(t, event.TransactionUUID.String(), got.GetTransactionUuid())
			assert.NotEmpty(t, got.GetEventUuid())

			return nil
		})

	require.NoError(t, orderkafka.NewOrderPaidPublisher(sender).PublishOrderPaid(context.Background(), event))
}

func shipAssembledMessage(t *testing.T, orderUUID string) *sarama.ConsumerMessage {
	t.Helper()

	payload, err := proto.Marshal(&eventsv1.ShipAssembled{OrderUuid: orderUUID, UserUuid: uuid.NewString()})
	require.NoError(t, err)

	return &sarama.ConsumerMessage{Topic: sharedkafka.TopicShipAssembled, Value: payload}
}

func TestShipAssembledHandler_MarksOrderAssembled(t *testing.T) {
	orderUUID := uuid.New()

	svc := mocks.NewMockAssemblyMarker(t)
	svc.EXPECT().MarkAssembled(context.Background(), orderUUID).Return(nil)

	handle := orderkafka.ShipAssembledHandler(svc)
	require.NoError(t, handle(context.Background(), shipAssembledMessage(t, orderUUID.String())))
}

func TestShipAssembledHandler_ServiceErrorIsRetried(t *testing.T) {
	orderUUID := uuid.New()

	svc := mocks.NewMockAssemblyMarker(t)
	svc.EXPECT().MarkAssembled(context.Background(), orderUUID).Return(errors.New("inventory down"))

	handle := orderkafka.ShipAssembledHandler(svc)
	require.Error(t, handle(context.Background(), shipAssembledMessage(t, orderUUID.String())))
}

func TestShipAssembledHandler_MalformedMessagesAreDropped(t *testing.T) {
	svc := mocks.NewMockAssemblyMarker(t) // must not be called

	handle := orderkafka.ShipAssembledHandler(svc)

	require.NoError(t, handle(context.Background(), &sarama.ConsumerMessage{Value: []byte{0xff, 0xff}}))
	require.NoError(t, handle(context.Background(), shipAssembledMessage(t, "not-a-uuid")))
}
