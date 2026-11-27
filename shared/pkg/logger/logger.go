// Package logger configures the process-wide slog logger: JSON to stdout and,
// unless telemetry is disabled, OTLP export through the OpenTelemetry log
// bridge (collector → Elasticsearch index otel-logs → Kibana).
package logger

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	sdklog "go.opentelemetry.io/otel/sdk/log"

	"github.com/omaigo88/shared/pkg/telemetry"
)

// Init installs the default slog logger for serviceName and returns a
// function that flushes buffered log records on shutdown.
//
// Use the *Context logging variants (slog.InfoContext, …) wherever a context
// is at hand: the OTel bridge then attaches the active trace and span IDs, so
// a log line in Kibana leads to its trace in Jaeger.
func Init(ctx context.Context, serviceName string) (telemetry.ShutdownFunc, error) {
	stdout := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})

	if telemetry.Disabled() {
		slog.SetDefault(slog.New(stdout).With("service", serviceName))

		return telemetry.NoopShutdown, nil
	}

	res, err := telemetry.Resource(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	exporter, err := otlploggrpc.New(ctx, otlploggrpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	)

	otelHandler := otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(provider))

	slog.SetDefault(slog.New(NewFanoutHandler(stdout.WithAttrs([]slog.Attr{slog.String("service", serviceName)}), otelHandler)))

	return provider.Shutdown, nil
}

// FanoutHandler sends every record to all of its handlers.
type FanoutHandler struct {
	handlers []slog.Handler
}

// NewFanoutHandler returns a handler that writes each record to every one of handlers.
func NewFanoutHandler(handlers ...slog.Handler) *FanoutHandler {
	return &FanoutHandler{handlers: handlers}
}

// Enabled reports whether any handler accepts records at level.
func (f *FanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}

	return false
}

// Handle passes a copy of r to every handler that accepts its level.
func (f *FanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error

	for _, h := range f.handlers {
		if h.Enabled(ctx, r.Level) {
			errs = append(errs, h.Handle(ctx, r.Clone()))
		}
	}

	return errors.Join(errs...)
}

// WithAttrs returns a FanoutHandler whose handlers all carry attrs.
func (f *FanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}

	return &FanoutHandler{handlers: handlers}
}

// WithGroup returns a FanoutHandler whose handlers all open group name.
func (f *FanoutHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		handlers[i] = h.WithGroup(name)
	}

	return &FanoutHandler{handlers: handlers}
}
