package metrics_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/omaigo88/order/pkg/metrics"
	"github.com/omaigo88/order/pkg/repository"
)

func collect(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Aggregation {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	out := map[string]metricdata.Aggregation{}

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			out[m.Name] = m.Data
		}
	}

	return out
}

func sumOf(t *testing.T, agg metricdata.Aggregation) map[attribute.Distinct]int64 {
	t.Helper()

	sum, ok := agg.(metricdata.Sum[int64])
	require.True(t, ok, "expected an int64 sum, got %T", agg)
	assert.True(t, sum.IsMonotonic)

	out := map[attribute.Distinct]int64{}
	for _, dp := range sum.DataPoints {
		out[dp.Attributes.Equivalent()] = dp.Value
	}

	return out
}

func attrs(kv ...attribute.KeyValue) attribute.Distinct {
	set := attribute.NewSet(kv...)

	return set.Equivalent()
}

func TestRecorder(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	rec, err := metrics.New(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	require.NoError(t, err)

	ctx := context.Background()
	rec.OrderCreated(ctx, 800000) // 8000 RUB
	rec.OrderCreated(ctx, 2700000)
	rec.OrderPaid(ctx, repository.PaymentMethodCard)
	rec.OrderPaid(ctx, repository.PaymentMethodCard)
	rec.OrderPaid(ctx, repository.PaymentMethodSBP)
	rec.OrderCancelled(ctx)
	rec.OrderAssembled(ctx, 12*time.Second)

	got := collect(t, reader)
	noAttrs := attrs()

	assert.Equal(t, int64(2), sumOf(t, got["orders_created"])[noAttrs])
	assert.Equal(t, int64(1), sumOf(t, got["orders_cancelled"])[noAttrs])
	assert.Equal(t, int64(1), sumOf(t, got["orders_assembled"])[noAttrs])

	paid := sumOf(t, got["orders_paid"])
	assert.Equal(t, int64(2), paid[attrs(attribute.String("payment_method", "CARD"))])
	assert.Equal(t, int64(1), paid[attrs(attribute.String("payment_method", "SBP"))])

	price, ok := got["order_total_price_rub"].(metricdata.Histogram[float64])
	require.True(t, ok)
	require.Len(t, price.DataPoints, 1)
	assert.Equal(t, uint64(2), price.DataPoints[0].Count)
	assert.InDelta(t, 35000.0, price.DataPoints[0].Sum, 0.001, "prices are recorded in roubles")

	fulfillment, ok := got["order_fulfillment_duration"].(metricdata.Histogram[float64])
	require.True(t, ok)
	assert.InDelta(t, 12.0, fulfillment.DataPoints[0].Sum, 0.001)
}
