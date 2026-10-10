package domain

import "fmt"

// CompatibilityChecker verifies that a set of parts can be assembled
// together. Currently the only rule: the engine must be rated to propel the
// hull's mass.
type CompatibilityChecker struct{}

// NewCompatibilityChecker creates a CompatibilityChecker.
func NewCompatibilityChecker() *CompatibilityChecker {
	return &CompatibilityChecker{}
}

// Check returns ErrIncompatibleParts if parts can't be assembled together.
// Parts other than a Hull/Engine pair don't participate in any rule yet.
func (CompatibilityChecker) Check(parts []Part) error {
	var hull, engine *Part

	for i := range parts {
		switch parts[i].Type() {
		case PartTypeHull:
			hull = &parts[i]
		case PartTypeEngine:
			engine = &parts[i]
		}
	}

	if hull == nil || engine == nil {
		return nil
	}

	hullProps := hull.Properties().Hull
	engineProps := engine.Properties().Engine

	if hullProps == nil || engineProps == nil {
		return nil
	}

	if engineProps.MaxHullMassKG < hullProps.MassKG {
		return fmt.Errorf("%w: engine %s is rated for %dkg, hull %s is %dkg",
			ErrIncompatibleParts, engine.ID(), engineProps.MaxHullMassKG, hull.ID(), hullProps.MassKG)
	}

	return nil
}
