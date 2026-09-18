// Package repository provides storage for spaceship parts.
package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
)

// ErrNotFound is returned when a part does not exist in the repository.
var ErrNotFound = errors.New("not found")

// Part represents a spaceship part.
type Part struct {
	UUID          string
	Name          string
	Description   string
	Price         int64 // in kopecks
	PartType      inventoryv1.PartType
	StockQuantity int64
	CreatedAt     *timestamppb.Timestamp
}

// Repository provides read access to the parts catalog.
type Repository interface {
	GetPart(ctx context.Context, id uuid.UUID) (Part, error)
	ListParts(ctx context.Context) ([]Part, error)
}
