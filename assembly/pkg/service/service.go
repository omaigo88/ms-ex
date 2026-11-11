// Package service contains the AssemblyService business logic: building a
// ship once its order has been paid.
package service

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
)

// OrderPaid is the input to assembly: a paid order and its owner.
type OrderPaid struct {
	OrderUUID uuid.UUID
	UserUUID  uuid.UUID
}

// ShipAssembled is the result of assembly.
type ShipAssembled struct {
	OrderUUID uuid.UUID
	UserUUID  uuid.UUID
	BuildTime time.Duration
}

// Assembler builds ships.
type Assembler struct {
	minBuildTime time.Duration
	maxBuildTime time.Duration
}

// NewAssembler creates an Assembler whose build time per ship is picked
// uniformly from [minBuildTime, maxBuildTime]. A max below min is treated as
// equal to min.
func NewAssembler(minBuildTime, maxBuildTime time.Duration) *Assembler {
	return &Assembler{minBuildTime: minBuildTime, maxBuildTime: max(minBuildTime, maxBuildTime)}
}

func (a *Assembler) pickBuildTime() time.Duration {
	spread := a.maxBuildTime - a.minBuildTime
	if spread <= 0 {
		return a.minBuildTime
	}

	return a.minBuildTime + rand.N(spread+1) //nolint:gosec // simulated delay, not security-sensitive
}

// Assemble builds the ship for a paid order, carrying the order and user
// through to the result. It returns ctx's error if cancelled mid-build, so an
// interrupted assembly isn't reported as done.
func (a *Assembler) Assemble(ctx context.Context, order OrderPaid) (ShipAssembled, error) {
	buildTime := a.pickBuildTime()

	timer := time.NewTimer(buildTime)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ShipAssembled{}, ctx.Err()
	case <-timer.C:
	}

	return ShipAssembled{
		OrderUUID: order.OrderUUID,
		UserUUID:  order.UserUUID,
		BuildTime: buildTime,
	}, nil
}
