package api_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/omaigo88/payment/pkg/api"
	"github.com/omaigo88/payment/pkg/api/mocks"
	"github.com/omaigo88/payment/pkg/service"
	paymentv1 "github.com/omaigo88/shared/pkg/proto/payment/v1"
)

func TestServer_PayOrder_Success(t *testing.T) {
	orderUUID := uuid.New().String()
	transactionUUID := uuid.New().String()

	svc := mocks.NewMockService(t)
	svc.EXPECT().
		PayOrder(context.Background(), orderUUID, paymentv1.PaymentMethod_PAYMENT_METHOD_CARD).
		Return(transactionUUID, nil)

	server := api.NewServer(svc)

	resp, err := server.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     orderUUID,
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.NoError(t, err)
	assert.Equal(t, transactionUUID, resp.GetTransactionUuid())
}

func TestServer_PayOrder_ValidationError(t *testing.T) {
	svc := mocks.NewMockService(t)
	svc.EXPECT().
		PayOrder(context.Background(), "", paymentv1.PaymentMethod_PAYMENT_METHOD_CARD).
		Return("", service.ErrEmptyOrderUUID)

	server := api.NewServer(svc)

	_, err := server.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     "",
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}
