package telemetry_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omaigo88/shared/pkg/telemetry"
)

func TestDisabled(t *testing.T) {
	t.Setenv("OTEL_SDK_DISABLED", "true")
	assert.True(t, telemetry.Disabled())

	t.Setenv("OTEL_SDK_DISABLED", "false")
	assert.False(t, telemetry.Disabled())

	t.Setenv("OTEL_SDK_DISABLED", "")
	assert.False(t, telemetry.Disabled())
}

func TestResource_HasServiceName(t *testing.T) {
	res, err := telemetry.Resource(context.Background(), "order-service")
	require.NoError(t, err)

	v, ok := res.Set().Value("service.name")
	require.True(t, ok)
	assert.Equal(t, "order-service", v.AsString())
}

func TestFlush_ReverseOrderAndKeepsGoingOnError(t *testing.T) {
	var order []string

	shutdown := func(name string, err error) telemetry.ShutdownFunc {
		return func(ctx context.Context) error {
			_, hasDeadline := ctx.Deadline()
			assert.True(t, hasDeadline)

			order = append(order, name)

			return err
		}
	}

	telemetry.Flush(shutdown("logger", nil), shutdown("tracer", errors.New("boom")), shutdown("meter", nil))

	assert.Equal(t, []string{"meter", "tracer", "logger"}, order)
}

func TestInit_FlushesStartedOnesOnFailure(t *testing.T) {
	var flushed []string

	ok := func(name string) telemetry.InitFunc {
		return func(context.Context, string) (telemetry.ShutdownFunc, error) {
			return func(context.Context) error {
				flushed = append(flushed, name)

				return nil
			}, nil
		}
	}
	failing := func(context.Context, string) (telemetry.ShutdownFunc, error) {
		return nil, errors.New("boom")
	}

	_, err := telemetry.Init(context.Background(), "svc", ok("logger"), failing, ok("never"))
	require.Error(t, err)
	assert.Equal(t, []string{"logger"}, flushed)

	shutdowns, err := telemetry.Init(context.Background(), "svc", ok("a"), ok("b"))
	require.NoError(t, err)
	assert.Len(t, shutdowns, 2)
}
