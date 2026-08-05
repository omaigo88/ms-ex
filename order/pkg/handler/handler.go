package handler

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	orderv1 "github.com/omaigo88/shared/pkg/openapi/order/v1"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
	paymentv1 "github.com/omaigo88/shared/pkg/proto/payment/v1"
)

// OrderStatus — статус заказа
type OrderStatus string

const (
	OrderStatusPendingPayment OrderStatus = "PENDING_PAYMENT"
	OrderStatusPaid           OrderStatus = "PAID"
	OrderStatusCancelled      OrderStatus = "CANCELLED"
)

// PaymentMethod — способ оплаты заказа
type PaymentMethod string

const (
	PaymentMethodCard          PaymentMethod = "CARD"
	PaymentMethodSBP           PaymentMethod = "SBP"
	PaymentMethodCreditCard    PaymentMethod = "CREDIT_CARD"
	PaymentMethodInvestorMoney PaymentMethod = "INVESTOR_MONEY"
)

// Order представляет заказ на постройку космического корабля
type Order struct {
	OrderUUID       uuid.UUID
	HullUUID        uuid.UUID
	EngineUUID      uuid.UUID
	ShieldUUID      *uuid.UUID // опциональный
	WeaponUUID      *uuid.UUID // опциональный
	TotalPrice      int64      // в копейках
	TransactionUUID *uuid.UUID
	PaymentMethod   *PaymentMethod
	Status          OrderStatus
	CreatedAt       time.Time
}

// orderStore — хранилище заказов (in-memory)
type orderStore struct {
	mu     sync.RWMutex
	orders map[uuid.UUID]Order
}

// NewOrderStore создаёт новое пустое хранилище заказов
func NewOrderStore() *orderStore {
	return &orderStore{
		orders: make(map[uuid.UUID]Order),
	}
}

// handler реализует интерфейс orderv1.Handler, сгенерированный ogen
type handler struct {
	orderv1.UnimplementedHandler
	inventoryClient inventoryv1.InventoryServiceClient
	paymentClient   paymentv1.PaymentServiceClient
	store           *orderStore
}

// NewHandler создаёт новый обработчик заказов
func NewHandler(
	inventoryClient inventoryv1.InventoryServiceClient,
	paymentClient paymentv1.PaymentServiceClient,
	store *orderStore,
) *handler {
	return &handler{
		inventoryClient: inventoryClient,
		paymentClient:   paymentClient,
		store:           store,
	}
}

// SetupServer создаёт OpenAPI сервер на основе обработчика
func SetupServer(h *handler) (*orderv1.Server, error) {
	return orderv1.NewServer(h)
}

// GetOrder реализует операцию getOrder (пример реализации)
// GET /api/v1/orders/{order_uuid}.
func (h *handler) GetOrder(_ context.Context, params orderv1.GetOrderParams) (orderv1.GetOrderRes, error) {
	// 1. Найти заказ в store (с блокировкой для thread-safety)
	h.store.mu.RLock()
	order, ok := h.store.orders[params.OrderUUID]
	h.store.mu.RUnlock()

	// 2. Если не найден — вернуть 404
	if !ok {
		return &orderv1.GetOrderNotFound{
			Code:    http.StatusNotFound,
			Message: "заказ не найден",
		}, nil
	}

	// 3. Преобразовать в DTO и вернуть
	var shieldUUID orderv1.OptNilUUID
	if order.ShieldUUID != nil {
		shieldUUID = orderv1.NewOptNilUUID(*order.ShieldUUID)
	}

	var weaponUUID orderv1.OptNilUUID
	if order.WeaponUUID != nil {
		weaponUUID = orderv1.NewOptNilUUID(*order.WeaponUUID)
	}

	var transactionUUID orderv1.OptNilUUID
	if order.TransactionUUID != nil {
		transactionUUID = orderv1.NewOptNilUUID(*order.TransactionUUID)
	}

	var paymentMethod orderv1.OptNilPaymentMethod
	if order.PaymentMethod != nil {
		paymentMethod = orderv1.NewOptNilPaymentMethod(orderv1.PaymentMethod(*order.PaymentMethod))
	}

	return &orderv1.OrderDto{
		OrderUUID:       order.OrderUUID,
		HullUUID:        order.HullUUID,
		EngineUUID:      order.EngineUUID,
		ShieldUUID:      shieldUUID,
		WeaponUUID:      weaponUUID,
		TotalPrice:      order.TotalPrice,
		TransactionUUID: transactionUUID,
		PaymentMethod:   paymentMethod,
		Status:          orderv1.OrderStatus(order.Status),
		CreatedAt:       order.CreatedAt,
	}, nil
}

// paymentMethodToProto преобразует orderv1.PaymentMethod в paymentv1.PaymentMethod
func paymentMethodToProto(pm orderv1.PaymentMethod) paymentv1.PaymentMethod {
	switch pm {
	case orderv1.PaymentMethodCARD:
		return paymentv1.PaymentMethod_PAYMENT_METHOD_CARD
	case orderv1.PaymentMethodSBP:
		return paymentv1.PaymentMethod_PAYMENT_METHOD_SBP
	case orderv1.PaymentMethodCREDITCARD:
		return paymentv1.PaymentMethod_PAYMENT_METHOD_CREDIT_CARD
	case orderv1.PaymentMethodINVESTORMONEY:
		return paymentv1.PaymentMethod_PAYMENT_METHOD_INVESTOR_MONEY
	default:
		return paymentv1.PaymentMethod_PAYMENT_METHOD_UNSPECIFIED
	}
}

// CreateOrder реализует операцию createOrder
// POST /api/v1/orders
func (h *handler) CreateOrder(ctx context.Context, req *orderv1.CreateOrderRequest) (orderv1.CreateOrderRes, error) {
	uuids := []string{req.GetHullUUID().String(), req.GetEngineUUID().String()}

	shieldUUID, hasShield := req.GetShieldUUID().Get()
	if hasShield {
		uuids = append(uuids, shieldUUID.String())
	}

	weaponUUID, hasWeapon := req.GetWeaponUUID().Get()
	if hasWeapon {
		uuids = append(uuids, weaponUUID.String())
	}

	resp, err := h.inventoryClient.ListParts(ctx, &inventoryv1.ListPartsRequest{Uuids: uuids})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			return &orderv1.CreateOrderNotFound{
				Code:    http.StatusNotFound,
				Message: st.Message(),
			}, nil
		}

		return nil, err
	}

	var totalPrice int64

	for _, part := range resp.GetParts() {
		if part.GetStockQuantity() <= 0 {
			return &orderv1.CreateOrderConflict{
				Code:    http.StatusConflict,
				Message: fmt.Sprintf("деталь %s отсутствует на складе", part.GetUuid()),
			}, nil
		}

		totalPrice += part.GetPrice()
	}

	order := Order{
		OrderUUID:  uuid.New(),
		HullUUID:   req.GetHullUUID(),
		EngineUUID: req.GetEngineUUID(),
		TotalPrice: totalPrice,
		Status:     OrderStatusPendingPayment,
		CreatedAt:  time.Now(),
	}

	if hasShield {
		order.ShieldUUID = &shieldUUID
	}

	if hasWeapon {
		order.WeaponUUID = &weaponUUID
	}

	h.store.mu.Lock()
	h.store.orders[order.OrderUUID] = order
	h.store.mu.Unlock()

	return &orderv1.CreateOrderResponse{
		OrderUUID:  order.OrderUUID,
		TotalPrice: order.TotalPrice,
	}, nil
}

// PayOrder реализует операцию payOrder
// POST /api/v1/orders/{order_uuid}/pay
func (h *handler) PayOrder(ctx context.Context, req *orderv1.PayOrderRequest, params orderv1.PayOrderParams) (orderv1.PayOrderRes, error) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()

	order, ok := h.store.orders[params.OrderUUID]
	if !ok {
		return &orderv1.PayOrderNotFound{
			Code:    http.StatusNotFound,
			Message: "заказ не найден",
		}, nil
	}

	if order.Status != OrderStatusPendingPayment {
		return &orderv1.PayOrderConflict{
			Code:    http.StatusConflict,
			Message: "заказ уже оплачен или отменён",
		}, nil
	}

	resp, err := h.paymentClient.PayOrder(ctx, &paymentv1.PayOrderRequest{
		OrderUuid:     order.OrderUUID.String(),
		PaymentMethod: paymentMethodToProto(req.GetPaymentMethod()),
	})
	if err != nil {
		return nil, err
	}

	transactionUUID, err := uuid.Parse(resp.GetTransactionUuid())
	if err != nil {
		return nil, err
	}

	paymentMethod := PaymentMethod(req.GetPaymentMethod())
	order.Status = OrderStatusPaid
	order.TransactionUUID = &transactionUUID
	order.PaymentMethod = &paymentMethod
	h.store.orders[order.OrderUUID] = order

	return &orderv1.PayOrderResponse{
		TransactionUUID: transactionUUID,
	}, nil
}

// CancelOrder реализует операцию cancelOrder
// POST /api/v1/orders/{order_uuid}/cancel
func (h *handler) CancelOrder(ctx context.Context, params orderv1.CancelOrderParams) (orderv1.CancelOrderRes, error) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()

	order, ok := h.store.orders[params.OrderUUID]
	if !ok {
		return &orderv1.CancelOrderNotFound{
			Code:    http.StatusNotFound,
			Message: "заказ не найден",
		}, nil
	}

	if order.Status != OrderStatusPendingPayment {
		return &orderv1.CancelOrderConflict{
			Code:    http.StatusConflict,
			Message: "заказ уже оплачен или отменён",
		}, nil
	}

	order.Status = OrderStatusCancelled
	h.store.orders[order.OrderUUID] = order

	return &orderv1.CancelOrderResponse{}, nil
}
