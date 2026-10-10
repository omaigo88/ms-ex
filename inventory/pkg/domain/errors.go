package domain

import "errors"

var (
	// ErrInvalidProperties is returned when a Part's properties don't match its PartType.
	ErrInvalidProperties = errors.New("properties do not match part type")
	// ErrInvalidQuantity is returned when a Reserve/Release quantity isn't positive.
	ErrInvalidQuantity = errors.New("quantity must be positive")
	// ErrInsufficientStock is returned when a Reserve would exceed available stock.
	ErrInsufficientStock = errors.New("insufficient stock")
	// ErrReservationUnderflow is returned when a Release would free more than is reserved.
	ErrReservationUnderflow = errors.New("cannot release more than reserved")
	// ErrIncompatibleParts is returned when a set of parts can't be assembled together.
	ErrIncompatibleParts = errors.New("incompatible parts")
)
