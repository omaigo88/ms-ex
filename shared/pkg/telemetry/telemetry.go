// Package telemetry holds what the logger, metrics and tracing packages share:
// the OpenTelemetry resource describing the service and the switch that turns
// exporting off.
package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// ShutdownFunc flushes and stops a telemetry provider.
type ShutdownFunc func(context.Context) error

// NoopShutdown is returned when exporting is disabled.
func NoopShutdown(context.Context) error { return nil }

// InitFunc initialises one signal (logger.Init, tracing.InitTracer, metrics.Init).
type InitFunc func(ctx context.Context, serviceName string) (ShutdownFunc, error)

// Init runs inits in order for serviceName and returns their shutdown
// functions for Flush. If one fails, the ones already started are flushed.
func Init(ctx context.Context, serviceName string, inits ...InitFunc) ([]ShutdownFunc, error) {
	shutdowns := make([]ShutdownFunc, 0, len(inits))

	for _, init := range inits {
		shutdown, err := init(ctx, serviceName)
		if err != nil {
			Flush(shutdowns...)

			return nil, fmt.Errorf("init telemetry: %w", err)
		}

		shutdowns = append(shutdowns, shutdown)
	}

	return shutdowns, nil
}

// flushTimeout bounds how long Flush waits for the collector on exit.
const flushTimeout = 5 * time.Second

// Flush runs the shutdown functions in reverse order (the logger, initialised
// first, is flushed last so it can still export what the others log), giving
// all of them together at most flushTimeout. Failures are logged, not returned:
// losing the last few seconds of telemetry must not fail a shutdown.
func Flush(shutdowns ...ShutdownFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()

	for i := len(shutdowns) - 1; i >= 0; i-- {
		if err := shutdowns[i](ctx); err != nil {
			slog.Warn("telemetry flush failed", "error", err)
		}
	}
}

// Disabled reports whether exporting is switched off with the standard
// OTEL_SDK_DISABLED=true (set by `task up`, which runs without a collector).
// The OTLP endpoint itself comes from the standard OTEL_EXPORTER_OTLP_ENDPOINT
// (default localhost:4317), read by the exporters.
func Disabled() bool {
	disabled, _ := strconv.ParseBool(os.Getenv("OTEL_SDK_DISABLED"))

	return disabled
}

// Resource describes serviceName for every signal it emits.
func Resource(ctx context.Context, serviceName string) (*resource.Resource, error) {
	env := os.Getenv("DEPLOYMENT_ENVIRONMENT")
	if env == "" {
		env = "local"
	}

	return resource.New(
		ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			attribute.String("deployment.environment.name", env),
		),
		resource.WithFromEnv(),
		resource.WithHost(),
	)
}
