// Package metrics configures the process-wide OpenTelemetry MeterProvider,
// exporting over OTLP (collector → Prometheus remote write → Grafana).
package metrics

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/omaigo88/shared/pkg/telemetry"
)

// exportInterval is how often metrics are pushed to the collector.
const exportInterval = 10 * time.Second

// Init installs the global MeterProvider for serviceName and returns a
// function that flushes pending metrics on shutdown. With telemetry disabled
// the global no-op provider stays in place, so instruments still work.
func Init(ctx context.Context, serviceName string) (telemetry.ShutdownFunc, error) {
	if telemetry.Disabled() {
		return telemetry.NoopShutdown, nil
	}

	res, err := telemetry.Resource(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	exporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(exportInterval))),
	)

	otel.SetMeterProvider(provider)

	return provider.Shutdown, nil
}
