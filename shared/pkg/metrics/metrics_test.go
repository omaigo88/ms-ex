package metrics_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/omaigo88/shared/pkg/metrics"
)

func restoreProvider(t *testing.T) {
	t.Helper()

	prev := otel.GetMeterProvider()
	t.Cleanup(func() { otel.SetMeterProvider(prev) })
}

func TestInit_DisabledKeepsGlobalProvider(t *testing.T) {
	restoreProvider(t)
	t.Setenv("OTEL_SDK_DISABLED", "true")

	before := otel.GetMeterProvider()

	shutdown, err := metrics.Init(context.Background(), "test-service")
	require.NoError(t, err)
	require.NoError(t, shutdown(context.Background()))
	assert.Equal(t, before, otel.GetMeterProvider())
}

func TestInit_EnabledInstallsSDKProvider(t *testing.T) {
	restoreProvider(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")

	shutdown, err := metrics.Init(context.Background(), "test-service")
	require.NoError(t, err)
	assert.IsType(t, &sdkmetric.MeterProvider{}, otel.GetMeterProvider())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = shutdown(ctx)
}
