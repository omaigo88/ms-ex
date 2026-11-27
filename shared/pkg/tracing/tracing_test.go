package tracing_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/omaigo88/shared/pkg/tracing"
)

func restoreGlobals(t *testing.T) {
	t.Helper()

	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})
}

func TestInitTracer_DisabledStillPropagates(t *testing.T) {
	restoreGlobals(t)
	t.Setenv("OTEL_SDK_DISABLED", "true")

	shutdown, err := tracing.InitTracer(context.Background(), "test-service")
	require.NoError(t, err)
	require.NoError(t, shutdown(context.Background()))

	assert.ElementsMatch(t, []string{"traceparent", "tracestate", "baggage"}, otel.GetTextMapPropagator().Fields())
}

func TestInitTracer_EnabledInstallsSDKProvider(t *testing.T) {
	restoreGlobals(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")

	shutdown, err := tracing.InitTracer(context.Background(), "test-service")
	require.NoError(t, err)
	assert.IsType(t, &sdktrace.TracerProvider{}, otel.GetTracerProvider())

	_, span := otel.Tracer("test").Start(context.Background(), "span")
	assert.True(t, span.SpanContext().IsValid(), "SDK provider records real spans")
	span.End()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = shutdown(ctx)
}
