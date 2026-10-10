// Package service contains the OrderService business logic.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	// ErrIncompatibleComponents is returned when the requested components can't be assembled together.
	ErrIncompatibleComponents = errors.New("components are incompatible")
)

// InventoryClient is the subset of inventoryv1.InventoryServiceClient used by Service.
type InventoryClient interface {
	ListParts(ctx context.Context, in *inventoryv1.ListPartsRequest, opts ...grpc.CallOption) (*inventoryv1.ListPartsResponse, error)
	ReserveParts(ctx context.Context, in *inventoryv1.ReservePartsRequest, opts ...grpc.CallOption) (*inventoryv1.ReservePartsResponse, error)
	ReleaseParts(ctx context.Context, in *inventoryv1.ReleasePartsRequest, opts ...grpc.CallOption) (*inventoryv1.ReleasePartsResponse, error)
}

// PaymentClient is the subset of paymentv1.PaymentServiceClient used by Service.
type PaymentClient interface {
	PayOrder(ctx context.Context, in *paymentv1.PayOrderRequest, opts ...grpc.CallOption) (*paymentv1.PayOrderResponse, error)
}

// Repository is the persistence dependency required by Service.
type Repository interface {
	Save(ctx context.Context, order repository.Order) error
	Get(ctx context.Context, id uuid.UUID) (repository.Order, error)
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
	trManager       TrManager
}

// NewService creates a new Service.
func NewService(inventoryClient InventoryClient, paymentClient PaymentClient, repo Repository, trManager TrManager) *Service {
	return &Service{
		inventoryClient: inventoryClient,
		paymentClient:   paymentClient,
		repo:            repo,
		trManager:       trManager,
	}
}

// mapInventoryErr translates a gRPC error from Inventory into an order
// domain sentinel error.
func mapInventoryErr(err error) error {
	st, ok := status.FromError(err)
	if !ok {
		return err
	}

	switch st.Code() {
	case codes.NotFound:
		return fmt.Errorf("%w: %s", ErrComponentNotFound, st.Message())
	case codes.FailedPrecondition:
		if strings.Contains(st.Message(), "incompatible") {
			return fmt.Errorf("%w: %s", ErrIncompatibleComponents, st.Message())
		}

		return fmt.Errorf("%w: %s", ErrComponentOutOfStock, st.Message())
	default:
		return err
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
		return repository.Order{}, mapInventoryErr(err)
	}

	var totalPrice int64
	for _, part := range resp.GetParts() {
		totalPrice += part.GetPrice()
	}

	// Reserve atomically as a set: stock and cross-part compatibility are
	// both enforced by Inventory before anything is committed there.
	if _, err := s.inventoryClient.ReserveParts(ctx, &inventoryv1.ReservePartsRequest{Uuids: uuids}); err != nil {
		return repository.Order{}, mapInventoryErr(err)
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

	if err := s.trManager.Do(ctx, func(ctx context.Context) error {
		return s.repo.Save(ctx, order)
	}); err != nil {
		return repository.Order{}, err
	}

	return order, nil
}

// GetOrder returns an order by UUID.
func (s *Service) GetOrder(ctx context.Context, id uuid.UUID) (repository.Order, error) {
	order, err := s.repo.Get(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.Order{}, fmt.Errorf("%w: %s", ErrOrderNotFound, id)
	}

	if err != nil {
		return repository.Order{}, err
	}

	return order, nil
}

// PayOrder pays for a pending order and returns the updated order.
func (s *Service) PayOrder(ctx context.Context, id uuid.UUID, method repository.PaymentMethod) (repository.Order, error) {
	order, err := s.repo.Get(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.Order{}, fmt.Errorf("%w: %s", ErrOrderNotFound, id)
	}

	if err != nil {
		return repository.Order{}, err
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

	if err := s.trManager.Do(ctx, func(ctx context.Context) error {
		return s.repo.Save(ctx, order)
	}); err != nil {
		return repository.Order{}, err
	}

	return order, nil
}

// CancelOrder cancels a pending order and returns the updated order.
func (s *Service) CancelOrder(ctx context.Context, id uuid.UUID) (repository.Order, error) {
	order, err := s.repo.Get(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.Order{}, fmt.Errorf("%w: %s", ErrOrderNotFound, id)
	}

	if err != nil {
		return repository.Order{}, err
	}

	if order.Status != repository.OrderStatusPendingPayment {
		return repository.Order{}, fmt.Errorf("%w: %s", ErrOrderAlreadyFinal, id)
	}

	uuids := []string{order.HullUUID.String(), order.EngineUUID.String()}
	if order.ShieldUUID != nil {
		uuids = append(uuids, order.ShieldUUID.String())
	}

	if order.WeaponUUID != nil {
		uuids = append(uuids, order.WeaponUUID.String())
	}

	if _, err := s.inventoryClient.ReleaseParts(ctx, &inventoryv1.ReleasePartsRequest{Uuids: uuids}); err != nil {
		return repository.Order{}, mapInventoryErr(err)
	}

	order.Status = repository.OrderStatusCancelled

	if err := s.trManager.Do(ctx, func(ctx context.Context) error {
		return s.repo.Save(ctx, order)
	}); err != nil {
		return repository.Order{}, err
	}

	return order, nil
}
