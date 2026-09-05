package api_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omaigo88/order/pkg/api"
	"github.com/omaigo88/order/pkg/api/mocks"
	"github.com/omaigo88/order/pkg/repository"
	"github.com/omaigo88/order/pkg/service"
	orderv1 "github.com/omaigo88/shared/pkg/openapi/order/v1"
)

func TestHandler_GetOrder_Success(t *testing.T) {
	id := uuid.New()
	order := repository.Order{
		OrderUUID:  id,
		HullUUID:   uuid.New(),
		EngineUUID: uuid.New(),
		TotalPrice: 800000,
		Status:     repository.OrderStatusPendingPayment,
		CreatedAt:  time.Now(),
	}

	svc := mocks.NewMockService(t)
	svc.EXPECT().GetOrder(context.Background(), id).Return(order, nil)

	h := api.NewHandler(svc)

	res, err := h.GetOrder(context.Background(), orderv1.GetOrderParams{OrderUUID: id})
	require.NoError(t, err)

	dto, ok := res.(*orderv1.OrderDto)
	require.True(t, ok)
	assert.Equal(t, id, dto.OrderUUID)
}

func TestHandler_GetOrder_NotFound(t *testing.T) {
	id := uuid.New()

	svc := mocks.NewMockService(t)
	svc.EXPECT().GetOrder(context.Background(), id).Return(repository.Order{}, service.ErrOrderNotFound)

	h := api.NewHandler(svc)

	res, err := h.GetOrder(context.Background(), orderv1.GetOrderParams{OrderUUID: id})
	require.NoError(t, err)

	_, ok := res.(*orderv1.GetOrderNotFound)
	assert.True(t, ok)
}

func TestHandler_CreateOrder_Success(t *testing.T) {
	hullUUID, engineUUID := uuid.New(), uuid.New()
	order := repository.Order{
		OrderUUID:  uuid.New(),
		HullUUID:   hullUUID,
		EngineUUID: engineUUID,
		TotalPrice: 800000,
		Status:     repository.OrderStatusPendingPayment,
	}

	svc := mocks.NewMockService(t)
	svc.EXPECT().
		CreateOrder(context.Background(), service.CreateOrderInput{HullUUID: hullUUID, EngineUUID: engineUUID}).
		Return(order, nil)

	h := api.NewHandler(svc)

	req := &orderv1.CreateOrderRequest{HullUUID: hullUUID, EngineUUID: engineUUID}

	res, err := h.CreateOrder(context.Background(), req)
	require.NoError(t, err)

	resp, ok := res.(*orderv1.CreateOrderResponse)
	require.True(t, ok)
	assert.Equal(t, order.OrderUUID, resp.OrderUUID)
	assert.Equal(t, order.TotalPrice, resp.TotalPrice)
}

func TestHandler_CreateOrder_ComponentNotFound(t *testing.T) {
	hullUUID, engineUUID := uuid.New(), uuid.New()

	svc := mocks.NewMockService(t)
	svc.EXPECT().
		CreateOrder(context.Background(), service.CreateOrderInput{HullUUID: hullUUID, EngineUUID: engineUUID}).
		Return(repository.Order{}, service.ErrComponentNotFound)

	h := api.NewHandler(svc)

	req := &orderv1.CreateOrderRequest{HullUUID: hullUUID, EngineUUID: engineUUID}

	res, err := h.CreateOrder(context.Background(), req)
	require.NoError(t, err)

	_, ok := res.(*orderv1.CreateOrderNotFound)
	assert.True(t, ok)
}

func TestHandler_CreateOrder_OutOfStock(t *testing.T) {
	hullUUID, engineUUID := uuid.New(), uuid.New()

	svc := mocks.NewMockService(t)
	svc.EXPECT().
		CreateOrder(context.Background(), service.CreateOrderInput{HullUUID: hullUUID, EngineUUID: engineUUID}).
		Return(repository.Order{}, service.ErrComponentOutOfStock)

	h := api.NewHandler(svc)

	req := &orderv1.CreateOrderRequest{HullUUID: hullUUID, EngineUUID: engineUUID}

	res, err := h.CreateOrder(context.Background(), req)
	require.NoError(t, err)

	_, ok := res.(*orderv1.CreateOrderConflict)
	assert.True(t, ok)
}

func TestHandler_PayOrder_Success(t *testing.T) {
	id := uuid.New()
	transactionUUID := uuid.New()
	order := repository.Order{
		OrderUUID:       id,
		Status:          repository.OrderStatusPaid,
		TransactionUUID: &transactionUUID,
	}

	svc := mocks.NewMockService(t)
	svc.EXPECT().PayOrder(context.Background(), id, repository.PaymentMethodCard).Return(order, nil)

	h := api.NewHandler(svc)

	req := &orderv1.PayOrderRequest{PaymentMethod: orderv1.PaymentMethodCARD}
	res, err := h.PayOrder(context.Background(), req, orderv1.PayOrderParams{OrderUUID: id})
	require.NoError(t, err)

	resp, ok := res.(*orderv1.PayOrderResponse)
	require.True(t, ok)
	assert.Equal(t, transactionUUID, resp.TransactionUUID)
}

func TestHandler_PayOrder_NotFound(t *testing.T) {
	id := uuid.New()

	svc := mocks.NewMockService(t)
	svc.EXPECT().PayOrder(context.Background(), id, repository.PaymentMethodCard).Return(repository.Order{}, service.ErrOrderNotFound)

	h := api.NewHandler(svc)

	req := &orderv1.PayOrderRequest{PaymentMethod: orderv1.PaymentMethodCARD}
	res, err := h.PayOrder(context.Background(), req, orderv1.PayOrderParams{OrderUUID: id})
	require.NoError(t, err)

	_, ok := res.(*orderv1.PayOrderNotFound)
	assert.True(t, ok)
}

func TestHandler_PayOrder_AlreadyFinal(t *testing.T) {
	id := uuid.New()

	svc := mocks.NewMockService(t)
	svc.EXPECT().PayOrder(context.Background(), id, repository.PaymentMethodCard).Return(repository.Order{}, service.ErrOrderAlreadyFinal)

	h := api.NewHandler(svc)

	req := &orderv1.PayOrderRequest{PaymentMethod: orderv1.PaymentMethodCARD}
	res, err := h.PayOrder(context.Background(), req, orderv1.PayOrderParams{OrderUUID: id})
	require.NoError(t, err)

	_, ok := res.(*orderv1.PayOrderConflict)
	assert.True(t, ok)
}

func TestHandler_CancelOrder_Success(t *testing.T) {
	id := uuid.New()

	svc := mocks.NewMockService(t)
	svc.EXPECT().CancelOrder(context.Background(), id).Return(repository.Order{OrderUUID: id, Status: repository.OrderStatusCancelled}, nil)

	h := api.NewHandler(svc)

	res, err := h.CancelOrder(context.Background(), orderv1.CancelOrderParams{OrderUUID: id})
	require.NoError(t, err)

	_, ok := res.(*orderv1.CancelOrderResponse)
	assert.True(t, ok)
}

func TestHandler_CancelOrder_NotFound(t *testing.T) {
	id := uuid.New()

	svc := mocks.NewMockService(t)
	svc.EXPECT().CancelOrder(context.Background(), id).Return(repository.Order{}, service.ErrOrderNotFound)

	h := api.NewHandler(svc)

	res, err := h.CancelOrder(context.Background(), orderv1.CancelOrderParams{OrderUUID: id})
	require.NoError(t, err)

	_, ok := res.(*orderv1.CancelOrderNotFound)
	assert.True(t, ok)
}

func TestHandler_CancelOrder_AlreadyFinal(t *testing.T) {
	id := uuid.New()

	svc := mocks.NewMockService(t)
	svc.EXPECT().CancelOrder(context.Background(), id).Return(repository.Order{}, service.ErrOrderAlreadyFinal)

	h := api.NewHandler(svc)

	res, err := h.CancelOrder(context.Background(), orderv1.CancelOrderParams{OrderUUID: id})
	require.NoError(t, err)

	_, ok := res.(*orderv1.CancelOrderConflict)
	assert.True(t, ok)
}
