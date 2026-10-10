package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/omaigo88/inventory/pkg/domain"
)

func mustPart(t *testing.T, partType domain.PartType, props domain.PartProperties) domain.Part {
	t.Helper()

	part, err := domain.NewPart(uuid.New(), "n", "d", 1, partType, props, 10, 0, time.Now())
	require.NoError(t, err)

	return part
}

func TestCompatibilityChecker_Compatible(t *testing.T) {
	hull := mustPart(t, domain.PartTypeHull, domain.PartProperties{Hull: &domain.HullProperties{MassKG: 1000}})
	engine := mustPart(t, domain.PartTypeEngine, domain.PartProperties{Engine: &domain.EngineProperties{MaxHullMassKG: 1500}})

	checker := domain.NewCompatibilityChecker()
	require.NoError(t, checker.Check([]domain.Part{hull, engine}))
}

func TestCompatibilityChecker_Incompatible(t *testing.T) {
	hull := mustPart(t, domain.PartTypeHull, domain.PartProperties{Hull: &domain.HullProperties{MassKG: 3000}})
	engine := mustPart(t, domain.PartTypeEngine, domain.PartProperties{Engine: &domain.EngineProperties{MaxHullMassKG: 1500}})

	checker := domain.NewCompatibilityChecker()
	require.ErrorIs(t, checker.Check([]domain.Part{hull, engine}), domain.ErrIncompatibleParts)
}

func TestCompatibilityChecker_NoHullOrEngine(t *testing.T) {
	shield := mustPart(t, domain.PartTypeShield, domain.PartProperties{Shield: &domain.ShieldProperties{CapacityMJ: 1}})

	checker := domain.NewCompatibilityChecker()
	require.NoError(t, checker.Check([]domain.Part{shield}))
}
