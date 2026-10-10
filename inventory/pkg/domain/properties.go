package domain

// HullProperties is the Value Object for PartTypeHull.
type HullProperties struct {
	MassKG      int64 `json:"mass_kg"`
	ArmorRating int64 `json:"armor_rating"`
}

// EngineProperties is the Value Object for PartTypeEngine. MaxHullMassKG is
// the heaviest hull this engine is rated to propel.
type EngineProperties struct {
	ThrustNewton  int64 `json:"thrust_newton"`
	MaxHullMassKG int64 `json:"max_hull_mass_kg"`
}

// ShieldProperties is the Value Object for PartTypeShield.
type ShieldProperties struct {
	CapacityMJ           int64 `json:"capacity_mj"`
	RechargeRateMJPerSec int64 `json:"recharge_rate_mj_per_sec"`
}

// WeaponProperties is the Value Object for PartTypeWeapon.
type WeaponProperties struct {
	DamageRating int64   `json:"damage_rating"`
	FireRateHz   float64 `json:"fire_rate_hz"`
}

// PartProperties is a "pointer union" Value Object: exactly one field is
// non-nil, matching the owning Part's PartType. Its own JSON encoding is
// exactly the shape stored in the parts.properties JSONB column, e.g.
// {"hull": {"mass_kg": 1000, "armor_rating": 20}}.
type PartProperties struct {
	Hull   *HullProperties   `json:"hull,omitempty"`
	Engine *EngineProperties `json:"engine,omitempty"`
	Shield *ShieldProperties `json:"shield,omitempty"`
	Weapon *WeaponProperties `json:"weapon,omitempty"`
}

// matchesType reports whether exactly the field for t is set.
func (p PartProperties) matchesType(t PartType) bool {
	switch t {
	case PartTypeHull:
		return p.Hull != nil && p.Engine == nil && p.Shield == nil && p.Weapon == nil
	case PartTypeEngine:
		return p.Engine != nil && p.Hull == nil && p.Shield == nil && p.Weapon == nil
	case PartTypeShield:
		return p.Shield != nil && p.Hull == nil && p.Engine == nil && p.Weapon == nil
	case PartTypeWeapon:
		return p.Weapon != nil && p.Hull == nil && p.Engine == nil && p.Shield == nil
	default:
		return false
	}
}
