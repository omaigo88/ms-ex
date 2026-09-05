// Package service contains the OrderService business logic.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/omaigo88/order/pkg/repository"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
	paymentv1 "github.com/omaigo88/shared/pkg/proto/payment/v1"
)

var (
	// ErrOrderNotFound is returned when an order does not exist.
	ErrOrderNotFound = errors.New("order not found")
	// ErrOrderAlreadyFinal is returned when an order is not pending payment anymore.
	ErrOrderAlreadyFinal = errors.New("order is already paid or cancelled")
	// ErrComponentNotFound is returned when a requested component does not exist.
	ErrComponentNotFound = errors.New("component not found")
	// ErrComponentOutOfStock is returned when a requested component has no stock.
	ErrComponentOutOfStock = errors.New("component out of stock")
)

// InventoryClient is the subset of inventoryv1.InventoryServiceClient used by Service.
type InventoryClient interface {
	ListParts(ctx context.Context, in *inventoryv1.ListPartsRequest, opts ...grpc.CallOption) (*inventoryv1.ListPartsResponse, error)
}

// PaymentClient is the subset of paymentv1.PaymentServiceClient used by Service.
type PaymentClient interface {
	PayOrder(ctx context.Context, in *paymentv1.PayOrderRequest, opts ...grpc.CallOption) (*paymentv1.PayOrderResponse, error)
}

// Repository is the persistence dependency required by Service.
type Repository interface {
	Save(order repository.Order)
	Get(id uuid.UUID) (repository.Order, bool)
}

// CreateOrderInput holds the parameters for creating an order.
type CreateOrderInput struct {
	HullUUID   uuid.UUID
	EngineUUID uuid.UUID
	ShieldUUID *uuid.UUID
	WeaponUUID *uuid.UUID
}

// Service implements the OrderService business logic.
type Service struct {
	inventoryClient InventoryClient
	paymentClient   PaymentClient
	repo            Repository
}

// NewService creates a new Service.
func NewService(inventoryClient InventoryClient, paymentClient PaymentClient, repo Repository) *Service {
	return &Service{
		inventoryClient: inventoryClient,
		paymentClient:   paymentClient,
		repo:            repo,
	}
}

// paymentMethodToProto converts a repository.PaymentMethod to paymentv1.PaymentMethod.
func paymentMethodToProto(pm repository.PaymentMethod) paymentv1.PaymentMethod {
	switch pm {
	case repository.PaymentMethodCard:
		return paymentv1.PaymentMethod_PAYMENT_METHOD_CARD
	case repository.PaymentMethodSBP:
		return paymentv1.PaymentMethod_PAYMENT_METHOD_SBP
	case repository.PaymentMethodCreditCard:
		return paymentv1.PaymentMethod_PAYMENT_METHOD_CREDIT_CARD
	case repository.PaymentMethodInvestorMoney:
		return paymentv1.PaymentMethod_PAYMENT_METHOD_INVESTOR_MONEY
	default:
		return paymentv1.PaymentMethod_PAYMENT_METHOD_UNSPECIFIED
	}
}

// CreateOrder creates a new order after checking that every requested component
// exists and is in stock.
func (s *Service) CreateOrder(ctx context.Context, input CreateOrderInput) (repository.Order, error) {
	uuids := []string{input.HullUUID.String(), input.EngineUUID.String()}

	if input.ShieldUUID != nil {
		uuids = append(uuids, input.ShieldUUID.String())
	}

	if input.WeaponUUID != nil {
		uuids = append(uuids, input.WeaponUUID.String())
	}

	resp, err := s.inventoryClient.ListParts(ctx, &inventoryv1.ListPartsRequest{Uuids: uuids})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			return repository.Order{}, fmt.Errorf("%w: %s", ErrComponentNotFound, st.Message())
		}

		return repository.Order{}, err
	}

	var totalPrice int64

	for _, part := range resp.GetParts() {
		if part.GetStockQuantity() <= 0 {
			return repository.Order{}, fmt.Errorf("%w: %s", ErrComponentOutOfStock, part.GetUuid())
		}

		totalPrice += part.GetPrice()
	}

	order := repository.Order{
		OrderUUID:  uuid.New(),
		HullUUID:   input.HullUUID,
		EngineUUID: input.EngineUUID,
		ShieldUUID: input.ShieldUUID,
		WeaponUUID: input.WeaponUUID,
		TotalPrice: totalPrice,
		Status:     repository.OrderStatusPendingPayment,
		CreatedAt:  time.Now(),
	}

	s.repo.Save(order)

	return order, nil
}

// GetOrder returns an order by UUID.
func (s *Service) GetOrder(_ context.Context, id uuid.UUID) (repository.Order, error) {
	order, ok := s.repo.Get(id)
	if !ok {
		return repository.Order{}, fmt.Errorf("%w: %s", ErrOrderNotFound, id)
	}

	return order, nil
}

// PayOrder pays for a pending order and returns the updated order.
func (s *Service) PayOrder(ctx context.Context, id uuid.UUID, method repository.PaymentMethod) (repository.Order, error) {
	order, ok := s.repo.Get(id)
	if !ok {
		return repository.Order{}, fmt.Errorf("%w: %s", ErrOrderNotFound, id)
	}

	if order.Status != repository.OrderStatusPendingPayment {
		return repository.Order{}, fmt.Errorf("%w: %s", ErrOrderAlreadyFinal, id)
	}

	resp, err := s.paymentClient.PayOrder(ctx, &paymentv1.PayOrderRequest{
		OrderUuid:     order.OrderUUID.String(),
		PaymentMethod: paymentMethodToProto(method),
	})
	if err != nil {
		return repository.Order{}, err
	}

	transactionUUID, err := uuid.Parse(resp.GetTransactionUuid())
	if err != nil {
		return repository.Order{}, err
	}

	order.Status = repository.OrderStatusPaid
	order.TransactionUUID = &transactionUUID
	order.PaymentMethod = &method
	s.repo.Save(order)

	return order, nil
}

// CancelOrder cancels a pending order and returns the updated order.
func (s *Service) CancelOrder(_ context.Context, id uuid.UUID) (repository.Order, error) {
	order, ok := s.repo.Get(id)
	if !ok {
		return repository.Order{}, fmt.Errorf("%w: %s", ErrOrderNotFound, id)
	}

	if order.Status != repository.OrderStatusPendingPayment {
		return repository.Order{}, fmt.Errorf("%w: %s", ErrOrderAlreadyFinal, id)
	}

	order.Status = repository.OrderStatusCancelled
	s.repo.Save(order)

	return order, nil
}
