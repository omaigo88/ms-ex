package service_test

import (
	"context"
	"errors"
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

type deps struct {
	inv  *mocks.MockInventoryClient
	pay  *mocks.MockPaymentClient
	repo *mocks.MockRepository
	pub  *mocks.MockOrderPaidPublisher
	svc  *service.Service
}

func newDeps(t *testing.T) deps {
	t.Helper()

	d := deps{
		inv:  mocks.NewMockInventoryClient(t),
		pay:  mocks.NewMockPaymentClient(t),
		repo: mocks.NewMockRepository(t),
		pub:  mocks.NewMockOrderPaidPublisher(t),
	}
	d.svc = service.NewService(d.inv, d.pay, d.repo, service.NewNoopTrManager(), d.pub)

	return d
}

func orderWithStatus(st repository.OrderStatus) repository.Order {
	return repository.Order{
		OrderUUID:  uuid.New(),
		UserUUID:   uuid.New(),
		HullUUID:   uuid.New(),
		EngineUUID: uuid.New(),
		Status:     st,
	}
}

func partUUIDs(o repository.Order) []string {
	return []string{o.HullUUID.String(), o.EngineUUID.String()}
}

func savedWithStatus(st repository.OrderStatus) interface{} {
	return mock.MatchedBy(func(o repository.Order) bool { return o.Status == st })
}

// --- CreateOrder ---

func TestService_CreateOrder_Success(t *testing.T) {
	d := newDeps(t)
	userUUID, hullUUID, engineUUID := uuid.New(), uuid.New(), uuid.New()
	uuids := []string{hullUUID.String(), engineUUID.String()}

	d.inv.EXPECT().ListParts(context.Background(), &inventoryv1.ListPartsRequest{Uuids: uuids}).
		Return(&inventoryv1.ListPartsResponse{Parts: []*inventoryv1.Part{{Price: 500000}, {Price: 300000}}}, nil)
	d.inv.EXPECT().ReserveParts(context.Background(), &inventoryv1.ReservePartsRequest{Uuids: uuids}).
		Return(&inventoryv1.ReservePartsResponse{}, nil)
	d.repo.EXPECT().Save(context.Background(), mock.MatchedBy(func(o repository.Order) bool {
		return o.UserUUID == userUUID && o.Status == repository.OrderStatusPendingPayment
	})).Return(nil)

	order, err := d.svc.CreateOrder(context.Background(), service.CreateOrderInput{
		UserUUID: userUUID, HullUUID: hullUUID, EngineUUID: engineUUID,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(800000), order.TotalPrice)
	assert.Equal(t, userUUID, order.UserUUID)
}

func TestService_CreateOrder_InventoryErrors(t *testing.T) {
	tests := []struct {
		name       string
		listErr    error
		reserveErr error
		want       error
	}{
		{"component not found", status.Error(codes.NotFound, "part not found"), nil, service.ErrComponentNotFound},
		{"out of stock", nil, status.Error(codes.FailedPrecondition, "insufficient stock: p"), service.ErrComponentOutOfStock},
		{"incompatible", nil, status.Error(codes.FailedPrecondition, "incompatible parts: x"), service.ErrIncompatibleComponents},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newDeps(t)

			if tt.listErr != nil {
				d.inv.EXPECT().ListParts(context.Background(), mock.Anything).Return(nil, tt.listErr)
			} else {
				d.inv.EXPECT().ListParts(context.Background(), mock.Anything).Return(&inventoryv1.ListPartsResponse{}, nil)
				d.inv.EXPECT().ReserveParts(context.Background(), mock.Anything).Return(nil, tt.reserveErr)
			}

			_, err := d.svc.CreateOrder(context.Background(), service.CreateOrderInput{HullUUID: uuid.New(), EngineUUID: uuid.New()})
			require.ErrorIs(t, err, tt.want)
		})
	}
}

// --- GetOrder ---

func TestService_GetOrder_Success(t *testing.T) {
	d := newDeps(t)
	want := orderWithStatus(repository.OrderStatusPendingPayment)
	d.repo.EXPECT().Get(context.Background(), want.OrderUUID).Return(want, nil)

	got, err := d.svc.GetOrder(context.Background(), want.OrderUUID)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestService_GetOrder_NotFound(t *testing.T) {
	d := newDeps(t)
	id := uuid.New()
	d.repo.EXPECT().Get(context.Background(), id).Return(repository.Order{}, repository.ErrNotFound)

	_, err := d.svc.GetOrder(context.Background(), id)
	require.ErrorIs(t, err, service.ErrOrderNotFound)
}

// --- PayOrder ---

func TestService_PayOrder_Success_PublishesOrderPaid(t *testing.T) {
	d := newDeps(t)
	existing := orderWithStatus(repository.OrderStatusPendingPayment)
	transactionUUID := uuid.New()

	d.repo.EXPECT().GetForUpdate(context.Background(), existing.OrderUUID).Return(existing, nil)
	d.pay.EXPECT().PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     existing.OrderUUID.String(),
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	}).Return(&paymentv1.PayOrderResponse{TransactionUuid: transactionUUID.String()}, nil)
	d.repo.EXPECT().Save(context.Background(), savedWithStatus(repository.OrderStatusPaid)).Return(nil)
	d.pub.EXPECT().PublishOrderPaid(context.Background(), service.OrderPaidEvent{
		OrderUUID:       existing.OrderUUID,
		UserUUID:        existing.UserUUID,
		PaymentMethod:   repository.PaymentMethodCard,
		TransactionUUID: transactionUUID,
	}).Return(nil)

	order, err := d.svc.PayOrder(context.Background(), existing.OrderUUID, repository.PaymentMethodCard)
	require.NoError(t, err)
	assert.Equal(t, repository.OrderStatusPaid, order.Status)
	assert.Equal(t, transactionUUID, *order.TransactionUUID)
}

func TestService_PayOrder_PublishFailureDoesNotFailPayment(t *testing.T) {
	d := newDeps(t)
	existing := orderWithStatus(repository.OrderStatusPendingPayment)

	d.repo.EXPECT().GetForUpdate(context.Background(), existing.OrderUUID).Return(existing, nil)
	d.pay.EXPECT().PayOrder(context.Background(), mock.Anything).
		Return(&paymentv1.PayOrderResponse{TransactionUuid: uuid.NewString()}, nil)
	d.repo.EXPECT().Save(context.Background(), mock.Anything).Return(nil)
	d.pub.EXPECT().PublishOrderPaid(context.Background(), mock.Anything).Return(errors.New("broker down"))

	order, err := d.svc.PayOrder(context.Background(), existing.OrderUUID, repository.PaymentMethodCard)
	require.NoError(t, err)
	assert.Equal(t, repository.OrderStatusPaid, order.Status)
}

func TestService_PayOrder_PaymentFailureNotPublished(t *testing.T) {
	d := newDeps(t)
	existing := orderWithStatus(repository.OrderStatusPendingPayment)

	d.repo.EXPECT().GetForUpdate(context.Background(), existing.OrderUUID).Return(existing, nil)
	d.pay.EXPECT().PayOrder(context.Background(), mock.Anything).Return(nil, errors.New("payment down"))
	// No Save, no PublishOrderPaid expected: mocks fail the test if called.

	_, err := d.svc.PayOrder(context.Background(), existing.OrderUUID, repository.PaymentMethodCard)
	require.Error(t, err)
}

func TestService_PayOrder_NotFound(t *testing.T) {
	d := newDeps(t)
	id := uuid.New()
	d.repo.EXPECT().GetForUpdate(context.Background(), id).Return(repository.Order{}, repository.ErrNotFound)

	_, err := d.svc.PayOrder(context.Background(), id, repository.PaymentMethodCard)
	require.ErrorIs(t, err, service.ErrOrderNotFound)
}

// --- status rules shared by PayOrder and CancelOrder ---

var nonPendingStatuses = []struct {
	status repository.OrderStatus
	want   error
}{
	{repository.OrderStatusPaid, service.ErrOrderAlreadyPaid},
	{repository.OrderStatusCancelled, service.ErrOrderCancelled},
	{repository.OrderStatusAssembled, service.ErrOrderAssembled},
}

func TestService_PayOrder_RejectsNonPending(t *testing.T) {
	for _, tt := range nonPendingStatuses {
		t.Run(string(tt.status), func(t *testing.T) {
			d := newDeps(t)
			existing := orderWithStatus(tt.status)
			d.repo.EXPECT().GetForUpdate(context.Background(), existing.OrderUUID).Return(existing, nil)

			_, err := d.svc.PayOrder(context.Background(), existing.OrderUUID, repository.PaymentMethodCard)
			require.ErrorIs(t, err, tt.want)
		})
	}
}

// --- CancelOrder ---

func TestService_CancelOrder_Success_ReleasesReservation(t *testing.T) {
	d := newDeps(t)
	existing := orderWithStatus(repository.OrderStatusPendingPayment)

	d.repo.EXPECT().GetForUpdate(context.Background(), existing.OrderUUID).Return(existing, nil)
	d.inv.EXPECT().ReleaseParts(context.Background(), &inventoryv1.ReleasePartsRequest{Uuids: partUUIDs(existing)}).
		Return(&inventoryv1.ReleasePartsResponse{}, nil)
	d.repo.EXPECT().Save(context.Background(), savedWithStatus(repository.OrderStatusCancelled)).Return(nil)

	order, err := d.svc.CancelOrder(context.Background(), existing.OrderUUID)
	require.NoError(t, err)
	assert.Equal(t, repository.OrderStatusCancelled, order.Status)
}

func TestService_CancelOrder_NotFound(t *testing.T) {
	d := newDeps(t)
	id := uuid.New()
	d.repo.EXPECT().GetForUpdate(context.Background(), id).Return(repository.Order{}, repository.ErrNotFound)

	_, err := d.svc.CancelOrder(context.Background(), id)
	require.ErrorIs(t, err, service.ErrOrderNotFound)
}

// TestService_CancelOrder_RejectsNonPending covers the status table: only
// PENDING_PAYMENT can be cancelled; every other status is a domain error and
// nothing is released or saved.
func TestService_CancelOrder_RejectsNonPending(t *testing.T) {
	for _, tt := range nonPendingStatuses {
		t.Run(string(tt.status), func(t *testing.T) {
			d := newDeps(t)
			existing := orderWithStatus(tt.status)
			d.repo.EXPECT().GetForUpdate(context.Background(), existing.OrderUUID).Return(existing, nil)

			_, err := d.svc.CancelOrder(context.Background(), existing.OrderUUID)
			require.ErrorIs(t, err, tt.want)
		})
	}
}

// --- MarkAssembled ---

func TestService_MarkAssembled_CommitsPartsAndSaves(t *testing.T) {
	d := newDeps(t)
	existing := orderWithStatus(repository.OrderStatusPaid)

	d.repo.EXPECT().GetForUpdate(context.Background(), existing.OrderUUID).Return(existing, nil)
	d.inv.EXPECT().CommitParts(context.Background(), &inventoryv1.CommitPartsRequest{Uuids: partUUIDs(existing)}).
		Return(&inventoryv1.CommitPartsResponse{}, nil)
	d.repo.EXPECT().Save(context.Background(), savedWithStatus(repository.OrderStatusAssembled)).Return(nil)

	require.NoError(t, d.svc.MarkAssembled(context.Background(), existing.OrderUUID))
}

func TestService_MarkAssembled_AlreadyAssembledIsNoop(t *testing.T) {
	d := newDeps(t)
	existing := orderWithStatus(repository.OrderStatusAssembled)
	d.repo.EXPECT().GetForUpdate(context.Background(), existing.OrderUUID).Return(existing, nil)

	require.NoError(t, d.svc.MarkAssembled(context.Background(), existing.OrderUUID))
}

func TestService_MarkAssembled_SkipsNonPaid(t *testing.T) {
	for _, st := range []repository.OrderStatus{repository.OrderStatusPendingPayment, repository.OrderStatusCancelled} {
		t.Run(string(st), func(t *testing.T) {
			d := newDeps(t)
			existing := orderWithStatus(st)
			d.repo.EXPECT().GetForUpdate(context.Background(), existing.OrderUUID).Return(existing, nil)

			require.NoError(t, d.svc.MarkAssembled(context.Background(), existing.OrderUUID))
		})
	}
}

func TestService_MarkAssembled_UnknownOrderIsSkipped(t *testing.T) {
	d := newDeps(t)
	id := uuid.New()
	d.repo.EXPECT().GetForUpdate(context.Background(), id).Return(repository.Order{}, repository.ErrNotFound)

	require.NoError(t, d.svc.MarkAssembled(context.Background(), id))
}

func TestService_MarkAssembled_CommitFailureIsRetried(t *testing.T) {
	d := newDeps(t)
	existing := orderWithStatus(repository.OrderStatusPaid)

	d.repo.EXPECT().GetForUpdate(context.Background(), existing.OrderUUID).Return(existing, nil)
	d.inv.EXPECT().CommitParts(context.Background(), mock.Anything).Return(nil, errors.New("inventory down"))

	require.Error(t, d.svc.MarkAssembled(context.Background(), existing.OrderUUID))
}
