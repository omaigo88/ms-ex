package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omaigo88/shared/pkg/logger"
)

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	var m map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))

	return m
}

func TestFanoutHandler_WritesToEveryHandler(t *testing.T) {
	var a, b bytes.Buffer

	log := slog.New(logger.NewFanoutHandler(
		slog.NewJSONHandler(&a, nil),
		slog.NewJSONHandler(&b, &slog.HandlerOptions{Level: slog.LevelWarn}),
	))

	log.With("svc", "order").WithGroup("req").Info("hello", "id", 7)

	got := decode(t, &a)
	assert.Equal(t, "hello", got["msg"])
	assert.Equal(t, "order", got["svc"])
	assert.Equal(t, map[string]any{"id": float64(7)}, got["req"])
	assert.Zero(t, b.Len(), "Info is below the second handler's level")

	log.Warn("careful")
	assert.Equal(t, "careful", decode(t, &b)["msg"])
}

func TestFanoutHandler_Enabled(t *testing.T) {
	h := logger.NewFanoutHandler(
		slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError}),
		slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn}),
	)

	assert.False(t, h.Enabled(context.Background(), slog.LevelInfo))
	assert.True(t, h.Enabled(context.Background(), slog.LevelWarn))
}

func restoreDefault(t *testing.T) {
	t.Helper()

	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
}

func TestInit_Disabled(t *testing.T) {
	restoreDefault(t)
	t.Setenv("OTEL_SDK_DISABLED", "true")

	shutdown, err := logger.Init(context.Background(), "test-service")
	require.NoError(t, err)
	require.NoError(t, shutdown(context.Background()))
}

func TestInit_EnabledWithoutCollector(t *testing.T) {
	restoreDefault(t)
	// Nothing listens here: Init must still succeed (export is asynchronous)
	// and shutdown must give up within its deadline instead of hanging.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")

	shutdown, err := logger.Init(context.Background(), "test-service")
	require.NoError(t, err)

	slog.Info("goes nowhere")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	_ = shutdown(ctx)
	assert.Less(t, time.Since(start), 3*time.Second)
}
