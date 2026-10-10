package domain

// PartType identifies what kind of spaceship part a Part is. Deliberately
// not the generated inventoryv1.PartType — the domain layer must not depend
// on transport/proto types — but numerically identical to it so converting
// at the edges (api layer) is a plain cast.
type PartType int32

const (
	PartTypeUnspecified PartType = 0
	PartTypeHull        PartType = 1
	PartTypeEngine      PartType = 2
	PartTypeShield      PartType = 3
	PartTypeWeapon      PartType = 4
)
