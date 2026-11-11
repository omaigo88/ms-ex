// Package repository provides storage for orders.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned when an order does not exist in the repository.
var ErrNotFound = errors.New("not found")

// OrderStatus — order status.
type OrderStatus string

const (
	OrderStatusPendingPayment OrderStatus = "PENDING_PAYMENT"
	OrderStatusPaid           OrderStatus = "PAID"
	OrderStatusCancelled      OrderStatus = "CANCELLED"
	OrderStatusAssembled      OrderStatus = "ASSEMBLED"
)

// PaymentMethod — order payment method.
type PaymentMethod string

const (
	PaymentMethodCard          PaymentMethod = "CARD"
	PaymentMethodSBP           PaymentMethod = "SBP"
	PaymentMethodCreditCard    PaymentMethod = "CREDIT_CARD"
	PaymentMethodInvestorMoney PaymentMethod = "INVESTOR_MONEY"
)

// Order represents an order to build a spaceship.
type Order struct {
	OrderUUID       uuid.UUID
	UserUUID        uuid.UUID
	HullUUID        uuid.UUID
	EngineUUID      uuid.UUID
	ShieldUUID      *uuid.UUID // optional
	WeaponUUID      *uuid.UUID // optional
	TotalPrice      int64      // in kopecks
	TransactionUUID *uuid.UUID
	PaymentMethod   *PaymentMethod
	Status          OrderStatus
	CreatedAt       time.Time
}

// Repository provides storage for orders.
type Repository interface {
	Save(ctx context.Context, order Order) error
	Get(ctx context.Context, id uuid.UUID) (Order, error)
	// GetForUpdate is Get plus a row lock held until the surrounding
	// transaction ends (SELECT ... FOR UPDATE).
	GetForUpdate(ctx context.Context, id uuid.UUID) (Order, error)
}
