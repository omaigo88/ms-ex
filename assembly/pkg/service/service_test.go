package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omaigo88/assembly/pkg/service"
)

func TestAssembler_CarriesOrderAndUserThrough(t *testing.T) {
	order := service.OrderPaid{OrderUUID: uuid.New(), UserUUID: uuid.New()}

	ship, err := service.NewAssembler(time.Millisecond, time.Millisecond).Assemble(context.Background(), order)
	require.NoError(t, err)
	assert.Equal(t, order.OrderUUID, ship.OrderUUID)
	assert.Equal(t, order.UserUUID, ship.UserUUID)
	assert.Equal(t, time.Millisecond, ship.BuildTime)
}

func TestAssembler_CancelledMidBuild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.NewAssembler(time.Hour, time.Hour).Assemble(ctx, service.OrderPaid{OrderUUID: uuid.New()})
	require.ErrorIs(t, err, context.Canceled)
}

func TestAssembler_BuildTimeWithinRange(t *testing.T) {
	const minBuild, maxBuild = time.Millisecond, 5 * time.Millisecond

	assembler := service.NewAssembler(minBuild, maxBuild)

	for range 20 {
		ship, err := assembler.Assemble(context.Background(), service.OrderPaid{OrderUUID: uuid.New()})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, ship.BuildTime, minBuild)
		assert.LessOrEqual(t, ship.BuildTime, maxBuild)
	}
}

func TestAssembler_MaxBelowMinUsesMin(t *testing.T) {
	ship, err := service.NewAssembler(2*time.Millisecond, time.Millisecond).
		Assemble(context.Background(), service.OrderPaid{OrderUUID: uuid.New()})
	require.NoError(t, err)
	assert.Equal(t, 2*time.Millisecond, ship.BuildTime)
}
