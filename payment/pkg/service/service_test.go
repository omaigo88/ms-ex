package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omaigo88/payment/pkg/service"
	paymentv1 "github.com/omaigo88/shared/pkg/proto/payment/v1"
)

func TestService_PayOrder_Success(t *testing.T) {
	svc := service.NewService()

	transactionUUID, err := svc.PayOrder(context.Background(), uuid.New().String(), paymentv1.PaymentMethod_PAYMENT_METHOD_CARD)
	require.NoError(t, err)
	assert.NotEmpty(t, transactionUUID)

	_, err = uuid.Parse(transactionUUID)
	assert.NoError(t, err)
}

func TestService_PayOrder_UniqueTransactions(t *testing.T) {
	svc := service.NewService()
	orderUUID := uuid.New().String()

	tx1, err := svc.PayOrder(context.Background(), orderUUID, paymentv1.PaymentMethod_PAYMENT_METHOD_CARD)
	require.NoError(t, err)

	tx2, err := svc.PayOrder(context.Background(), orderUUID, paymentv1.PaymentMethod_PAYMENT_METHOD_CARD)
	require.NoError(t, err)

	assert.NotEqual(t, tx1, tx2)
}

func TestService_PayOrder_Validation(t *testing.T) {
	tests := []struct {
		name      string
		orderUUID string
		method    paymentv1.PaymentMethod
		wantErr   error
	}{
		{
			name:      "empty order uuid",
			orderUUID: "",
			method:    paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
			wantErr:   service.ErrEmptyOrderUUID,
		},
		{
			name:      "unspecified payment method",
			orderUUID: uuid.New().String(),
			method:    paymentv1.PaymentMethod_PAYMENT_METHOD_UNSPECIFIED,
			wantErr:   service.ErrUnspecifiedMethod,
		},
		{
			name:      "invalid order uuid format",
			orderUUID: "not-a-uuid",
			method:    paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
			wantErr:   service.ErrInvalidOrderUUID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := service.NewService()

			_, err := svc.PayOrder(context.Background(), tt.orderUUID, tt.method)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}
