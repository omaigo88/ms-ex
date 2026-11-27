package service

import (
	"context"
	"time"

	"github.com/omaigo88/order/pkg/repository"
)

// Metrics records business metrics. Each method is called only after the
// corresponding state change has been committed.
type Metrics interface {
	OrderCreated(ctx context.Context, totalPrice int64)
	OrderPaid(ctx context.Context, method repository.PaymentMethod)
	OrderCancelled(ctx context.Context)
	OrderAssembled(ctx context.Context, fulfillment time.Duration)
}

// noopMetrics discards everything. Used where no meter is wired up (the
// self-contained API test suite).
type noopMetrics struct{}

// NewNoopMetrics creates a Metrics that records nothing.
func NewNoopMetrics() Metrics {
	return noopMetrics{}
}

func (noopMetrics) OrderCreated(context.Context, int64)                 {}
func (noopMetrics) OrderPaid(context.Context, repository.PaymentMethod) {}
func (noopMetrics) OrderCancelled(context.Context)                      {}
func (noopMetrics) OrderAssembled(context.Context, time.Duration)       {}
