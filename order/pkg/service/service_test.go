package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/omaigo88/order/pkg/repository"
	"github.com/omaigo88/order/pkg/service"
	"github.com/omaigo88/order/pkg/service/mocks"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
	paymentv1 "github.com/omaigo88/shared/pkg/proto/payment/v1"
)

func notFoundGRPCErr() error {
	return status.Error(codes.NotFound, "part not found")
}

func TestService_CreateOrder_Success(t *testing.T) {
	hullUUID, engineUUID := uuid.New(), uuid.New()

	inv := mocks.NewMockInventoryClient(t)
	inv.EXPECT().
		ListParts(context.Background(), &inventoryv1.ListPartsRequest{Uuids: []string{hullUUID.String(), engineUUID.String()}}).
		Return(&inventoryv1.ListPartsResponse{
			Parts: []*inventoryv1.Part{
				{Uuid: hullUUID.String(), Price: 500000, StockQuantity: 1},
				{Uuid: engineUUID.String(), Price: 300000, StockQuantity: 1},
			},
		}, nil)

	pay := mocks.NewMockPaymentClient(t)
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Save(context.Background(), mock.Anything).Return(nil)

	svc := service.NewService(inv, pay, repo, service.NewNoopTrManager())

	order, err := svc.CreateOrder(context.Background(), service.CreateOrderInput{
		HullUUID:   hullUUID,
		EngineUUID: engineUUID,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(800000), order.TotalPrice)
	assert.Equal(t, repository.OrderStatusPendingPayment, order.Status)
}

func TestService_CreateOrder_ComponentNotFound(t *testing.T) {
	hullUUID, engineUUID := uuid.New(), uuid.New()

	inv := mocks.NewMockInventoryClient(t)
	inv.EXPECT().
		ListParts(context.Background(), &inventoryv1.ListPartsRequest{Uuids: []string{hullUUID.String(), engineUUID.String()}}).
		Return(nil, notFoundGRPCErr())

	pay := mocks.NewMockPaymentClient(t)
	repo := mocks.NewMockRepository(t)

	svc := service.NewService(inv, pay, repo, service.NewNoopTrManager())

	_, err := svc.CreateOrder(context.Background(), service.CreateOrderInput{
		HullUUID:   hullUUID,
		EngineUUID: engineUUID,
	})
	require.ErrorIs(t, err, service.ErrComponentNotFound)
}

func TestService_CreateOrder_OutOfStock(t *testing.T) {
	hullUUID, engineUUID := uuid.New(), uuid.New()

	inv := mocks.NewMockInventoryClient(t)
	inv.EXPECT().
		ListParts(context.Background(), &inventoryv1.ListPartsRequest{Uuids: []string{hullUUID.String(), engineUUID.String()}}).
		Return(&inventoryv1.ListPartsResponse{
			Parts: []*inventoryv1.Part{
				{Uuid: hullUUID.String(), Price: 500000, StockQuantity: 0},
				{Uuid: engineUUID.String(), Price: 300000, StockQuantity: 1},
			},
		}, nil)

	pay := mocks.NewMockPaymentClient(t)
	repo := mocks.NewMockRepository(t)

	svc := service.NewService(inv, pay, repo, service.NewNoopTrManager())

	_, err := svc.CreateOrder(context.Background(), service.CreateOrderInput{
		HullUUID:   hullUUID,
		EngineUUID: engineUUID,
	})
	require.ErrorIs(t, err, service.ErrComponentOutOfStock)
}

func TestService_GetOrder_Success(t *testing.T) {
	id := uuid.New()
	want := repository.Order{OrderUUID: id, Status: repository.OrderStatusPendingPayment}

	inv := mocks.NewMockInventoryClient(t)
	pay := mocks.NewMockPaymentClient(t)
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Get(context.Background(), id).Return(want, nil)

	svc := service.NewService(inv, pay, repo, service.NewNoopTrManager())

	got, err := svc.GetOrder(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestService_GetOrder_NotFound(t *testing.T) {
	id := uuid.New()

	inv := mocks.NewMockInventoryClient(t)
	pay := mocks.NewMockPaymentClient(t)
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Get(context.Background(), id).Return(repository.Order{}, repository.ErrNotFound)

	svc := service.NewService(inv, pay, repo, service.NewNoopTrManager())

	_, err := svc.GetOrder(context.Background(), id)
	require.ErrorIs(t, err, service.ErrOrderNotFound)
}

func TestService_PayOrder_Success(t *testing.T) {
	id := uuid.New()
	transactionUUID := uuid.New()
	existing := repository.Order{OrderUUID: id, Status: repository.OrderStatusPendingPayment}

	inv := mocks.NewMockInventoryClient(t)

	pay := mocks.NewMockPaymentClient(t)
	pay.EXPECT().
		PayOrder(context.Background(), &paymentv1.PayOrderRequest{
			OrderUuid:     id.String(),
			PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
		}).
		Return(&paymentv1.PayOrderResponse{TransactionUuid: transactionUUID.String()}, nil)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Get(context.Background(), id).Return(existing, nil)
	repo.EXPECT().Save(context.Background(), mock.Anything).Return(nil)

	svc := service.NewService(inv, pay, repo, service.NewNoopTrManager())

	order, err := svc.PayOrder(context.Background(), id, repository.PaymentMethodCard)
	require.NoError(t, err)
	assert.Equal(t, repository.OrderStatusPaid, order.Status)
	require.NotNil(t, order.TransactionUUID)
	assert.Equal(t, transactionUUID, *order.TransactionUUID)
}

func TestService_PayOrder_NotFound(t *testing.T) {
	id := uuid.New()

	inv := mocks.NewMockInventoryClient(t)
	pay := mocks.NewMockPaymentClient(t)
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Get(context.Background(), id).Return(repository.Order{}, repository.ErrNotFound)

	svc := service.NewService(inv, pay, repo, service.NewNoopTrManager())

	_, err := svc.PayOrder(context.Background(), id, repository.PaymentMethodCard)
	require.ErrorIs(t, err, service.ErrOrderNotFound)
}

func TestService_PayOrder_AlreadyFinal(t *testing.T) {
	id := uuid.New()
	existing := repository.Order{OrderUUID: id, Status: repository.OrderStatusPaid}

	inv := mocks.NewMockInventoryClient(t)
	pay := mocks.NewMockPaymentClient(t)
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Get(context.Background(), id).Return(existing, nil)

	svc := service.NewService(inv, pay, repo, service.NewNoopTrManager())

	_, err := svc.PayOrder(context.Background(), id, repository.PaymentMethodCard)
	require.ErrorIs(t, err, service.ErrOrderAlreadyFinal)
}

func TestService_CancelOrder_Success(t *testing.T) {
	id := uuid.New()
	existing := repository.Order{OrderUUID: id, Status: repository.OrderStatusPendingPayment}

	inv := mocks.NewMockInventoryClient(t)
	pay := mocks.NewMockPaymentClient(t)
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Get(context.Background(), id).Return(existing, nil)
	repo.EXPECT().Save(context.Background(), mock.Anything).Return(nil)

	svc := service.NewService(inv, pay, repo, service.NewNoopTrManager())

	order, err := svc.CancelOrder(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, repository.OrderStatusCancelled, order.Status)
}

func TestService_CancelOrder_NotFound(t *testing.T) {
	id := uuid.New()

	inv := mocks.NewMockInventoryClient(t)
	pay := mocks.NewMockPaymentClient(t)
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Get(context.Background(), id).Return(repository.Order{}, repository.ErrNotFound)

	svc := service.NewService(inv, pay, repo, service.NewNoopTrManager())

	_, err := svc.CancelOrder(context.Background(), id)
	require.ErrorIs(t, err, service.ErrOrderNotFound)
}

func TestService_CancelOrder_AlreadyFinal(t *testing.T) {
	id := uuid.New()
	existing := repository.Order{OrderUUID: id, Status: repository.OrderStatusCancelled}

	inv := mocks.NewMockInventoryClient(t)
	pay := mocks.NewMockPaymentClient(t)
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Get(context.Background(), id).Return(existing, nil)

	svc := service.NewService(inv, pay, repo, service.NewNoopTrManager())

	_, err := svc.CancelOrder(context.Background(), id)
	require.ErrorIs(t, err, service.ErrOrderAlreadyFinal)
}
