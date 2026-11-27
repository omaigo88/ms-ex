// Package tracing configures the process-wide OpenTelemetry TracerProvider,
// exporting over OTLP (collector → Jaeger), and the W3C propagators that
// carry trace context across HTTP and gRPC calls.
package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/omaigo88/shared/pkg/telemetry"
)

// InitTracer installs the global TracerProvider and propagator for
// serviceName and returns a function that flushes pending spans on shutdown.
// The propagator is installed even with telemetry disabled, so a disabled
// service still forwards incoming trace context to the services it calls.
func InitTracer(ctx context.Context, serviceName string) (telemetry.ShutdownFunc, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if telemetry.Disabled() {
		return telemetry.NoopShutdown, nil
	}

	res, err := telemetry.Resource(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
	)

	otel.SetTracerProvider(provider)

	return provider.Shutdown, nil
}
