// Package metrics implements OrderService's business metrics on the
// OpenTelemetry meter (exported to Prometheus through the collector).
package metrics

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/omaigo88/order/pkg/repository"
)

const meterName = "github.com/omaigo88/order"

// Recorder implements service.Metrics.
type Recorder struct {
	created     metric.Int64Counter
	paid        metric.Int64Counter
	cancelled   metric.Int64Counter
	assembled   metric.Int64Counter
	totalPrice  metric.Float64Histogram
	fulfillment metric.Float64Histogram
}

// New creates a Recorder whose instruments come from provider (normally
// otel.GetMeterProvider() after metrics.Init).
func New(provider metric.MeterProvider) (*Recorder, error) {
	meter := provider.Meter(meterName)

	var (
		r   Recorder
		err error
	)

	if r.created, err = meter.Int64Counter("orders_created",
		metric.WithDescription("Orders created")); err != nil {
		return nil, err
	}

	if r.paid, err = meter.Int64Counter("orders_paid",
		metric.WithDescription("Orders paid, by payment method")); err != nil {
		return nil, err
	}

	if r.cancelled, err = meter.Int64Counter("orders_cancelled",
		metric.WithDescription("Orders cancelled")); err != nil {
		return nil, err
	}

	if r.assembled, err = meter.Int64Counter("orders_assembled",
		metric.WithDescription("Orders whose ship was assembled")); err != nil {
		return nil, err
	}

	// The currency is part of the name rather than the unit: Prometheus
	// remote write appends units to names, and "RUB" isn't a UCUM unit.
	if r.totalPrice, err = meter.Float64Histogram("order_total_price_rub",
		metric.WithDescription("Total price of created orders, in roubles"),
		metric.WithExplicitBucketBoundaries(1000, 5000, 10000, 20000, 30000, 50000, 100000)); err != nil {
		return nil, err
	}

	if r.fulfillment, err = meter.Float64Histogram("order_fulfillment_duration",
		metric.WithDescription("Time from order creation to its ship being assembled"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(5, 10, 15, 30, 60, 120, 300, 600, 1800, 3600)); err != nil {
		return nil, err
	}

	return &r, nil
}

// OrderCreated records a created order and its total price (kopecks).
func (r *Recorder) OrderCreated(ctx context.Context, totalPrice int64) {
	r.created.Add(ctx, 1)
	r.totalPrice.Record(ctx, float64(totalPrice)/100)
}

// OrderPaid records a paid order.
func (r *Recorder) OrderPaid(ctx context.Context, method repository.PaymentMethod) {
	r.paid.Add(ctx, 1, metric.WithAttributes(attribute.String("payment_method", string(method))))
}

// OrderCancelled records a cancelled order.
func (r *Recorder) OrderCancelled(ctx context.Context) {
	r.cancelled.Add(ctx, 1)
}

// OrderAssembled records an assembled order and how long fulfilment took.
func (r *Recorder) OrderAssembled(ctx context.Context, fulfillment time.Duration) {
	r.assembled.Add(ctx, 1)
	r.fulfillment.Record(ctx, fulfillment.Seconds())
}
