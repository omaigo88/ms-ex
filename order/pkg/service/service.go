// Package service contains the OrderService business logic.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	// ErrOrderAlreadyPaid is returned when an operation needs a pending order but it's already paid.
	ErrOrderAlreadyPaid = errors.New("order is already paid")
	// ErrOrderCancelled is returned when an operation needs a pending order but it's cancelled.
	ErrOrderCancelled = errors.New("order is cancelled")
	// ErrOrderAssembled is returned when an operation needs a pending order but its ship is already assembled.
	ErrOrderAssembled = errors.New("order is already assembled")
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
	CommitParts(ctx context.Context, in *inventoryv1.CommitPartsRequest, opts ...grpc.CallOption) (*inventoryv1.CommitPartsResponse, error)
}

// PaymentClient is the subset of paymentv1.PaymentServiceClient used by Service.
type PaymentClient interface {
	PayOrder(ctx context.Context, in *paymentv1.PayOrderRequest, opts ...grpc.CallOption) (*paymentv1.PayOrderResponse, error)
}

// Repository is the persistence dependency required by Service.
type Repository interface {
	Save(ctx context.Context, order repository.Order) error
	Get(ctx context.Context, id uuid.UUID) (repository.Order, error)
	GetForUpdate(ctx context.Context, id uuid.UUID) (repository.Order, error)
}

// CreateOrderInput holds the parameters for creating an order.
type CreateOrderInput struct {
	UserUUID   uuid.UUID
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
	publisher       OrderPaidPublisher
}

// NewService creates a new Service.
func NewService(
	inventoryClient InventoryClient,
	paymentClient PaymentClient,
	repo Repository,
	trManager TrManager,
	publisher OrderPaidPublisher,
) *Service {
	return &Service{
		inventoryClient: inventoryClient,
		paymentClient:   paymentClient,
		repo:            repo,
		trManager:       trManager,
		publisher:       publisher,
	}
}

// statusErr returns the error for an operation that requires a
// PENDING_PAYMENT order, or nil if the order is pending.
func statusErr(order repository.Order) error {
	switch order.Status {
	case repository.OrderStatusPaid:
		return fmt.Errorf("%w: %s", ErrOrderAlreadyPaid, order.OrderUUID)
	case repository.OrderStatusCancelled:
		return fmt.Errorf("%w: %s", ErrOrderCancelled, order.OrderUUID)
	case repository.OrderStatusAssembled:
		return fmt.Errorf("%w: %s", ErrOrderAssembled, order.OrderUUID)
	default:
		return nil
	}
}

// partUUIDs lists every part an order is built from.
func partUUIDs(order repository.Order) []string {
	uuids := []string{order.HullUUID.String(), order.EngineUUID.String()}
	if order.ShieldUUID != nil {
		uuids = append(uuids, order.ShieldUUID.String())
	}

	if order.WeaponUUID != nil {
		uuids = append(uuids, order.WeaponUUID.String())
	}

	return uuids
}

// getForUpdate loads and row-locks an order; only meaningful inside s.trManager.Do.
func (s *Service) getForUpdate(ctx context.Context, id uuid.UUID) (repository.Order, error) {
	order, err := s.repo.GetForUpdate(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.Order{}, fmt.Errorf("%w: %s", ErrOrderNotFound, id)
	}

	return order, err
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
		UserUUID:   input.UserUUID,
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

// PayOrder pays for a pending order and returns the updated order. The whole
// read-check-pay-save sequence runs in one transaction on a locked row, so a
// concurrent pay or cancel of the same order waits instead of racing. Once it
// commits, OrderPaid is published to kick off assembly.
func (s *Service) PayOrder(ctx context.Context, id uuid.UUID, method repository.PaymentMethod) (repository.Order, error) {
	var paid repository.Order

	err := s.trManager.Do(ctx, func(ctx context.Context) error {
		order, err := s.getForUpdate(ctx, id)
		if err != nil {
			return err
		}

		if err := statusErr(order); err != nil {
			return err
		}

		resp, err := s.paymentClient.PayOrder(ctx, &paymentv1.PayOrderRequest{
			OrderUuid:     order.OrderUUID.String(),
			PaymentMethod: paymentMethodToProto(method),
		})
		if err != nil {
			return err
		}

		transactionUUID, err := uuid.Parse(resp.GetTransactionUuid())
		if err != nil {
			return err
		}

		order.Status = repository.OrderStatusPaid
		order.TransactionUUID = &transactionUUID
		order.PaymentMethod = &method

		if err := s.repo.Save(ctx, order); err != nil {
			return err
		}

		paid = order

		return nil
	})
	if err != nil {
		return repository.Order{}, err
	}

	// Published after commit, never for a rolled-back payment. A failure here
	// is logged rather than returned: the payment itself succeeded, and a 500
	// would only make the client retry into "already paid".
	if err := s.publisher.PublishOrderPaid(ctx, OrderPaidEvent{
		OrderUUID:       paid.OrderUUID,
		UserUUID:        paid.UserUUID,
		PaymentMethod:   method,
		TransactionUUID: *paid.TransactionUUID,
	}); err != nil {
		slog.Error("failed to publish OrderPaid", "order_uuid", paid.OrderUUID, "error", err)
	}

	return paid, nil
}

// CancelOrder cancels a pending order, releasing its reserved parts, and
// returns the updated order. Only PENDING_PAYMENT orders can be cancelled.
func (s *Service) CancelOrder(ctx context.Context, id uuid.UUID) (repository.Order, error) {
	var cancelled repository.Order

	err := s.trManager.Do(ctx, func(ctx context.Context) error {
		order, err := s.getForUpdate(ctx, id)
		if err != nil {
			return err
		}

		if err := statusErr(order); err != nil {
			return err
		}

		if _, err := s.inventoryClient.ReleaseParts(ctx, &inventoryv1.ReleasePartsRequest{Uuids: partUUIDs(order)}); err != nil {
			return mapInventoryErr(err)
		}

		order.Status = repository.OrderStatusCancelled

		if err := s.repo.Save(ctx, order); err != nil {
			return err
		}

		cancelled = order

		return nil
	})
	if err != nil {
		return repository.Order{}, err
	}

	return cancelled, nil
}

// MarkAssembled handles a ShipAssembled event: the order's reserved parts are
// written off in Inventory and the order moves to ASSEMBLED. Events are
// delivered at least once, so a repeat for an already-assembled order is a
// no-op; an event for an unknown or non-PAID order is logged and dropped
// rather than retried forever.
func (s *Service) MarkAssembled(ctx context.Context, id uuid.UUID) error {
	return s.trManager.Do(ctx, func(ctx context.Context) error {
		order, err := s.getForUpdate(ctx, id)
		if errors.Is(err, ErrOrderNotFound) {
			slog.Warn("ShipAssembled for unknown order, skipping", "order_uuid", id)

			return nil
		}

		if err != nil {
			return err
		}

		switch order.Status {
		case repository.OrderStatusAssembled:
			return nil
		case repository.OrderStatusPaid:
		default:
			slog.Warn("ShipAssembled for an order that isn't paid, skipping",
				"order_uuid", id, "status", order.Status)

			return nil
		}

		if _, err := s.inventoryClient.CommitParts(ctx, &inventoryv1.CommitPartsRequest{Uuids: partUUIDs(order)}); err != nil {
			return fmt.Errorf("commit parts: %w", err)
		}

		order.Status = repository.OrderStatusAssembled

		return s.repo.Save(ctx, order)
	})
}
