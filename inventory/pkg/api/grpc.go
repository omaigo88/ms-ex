// Package api implements the InventoryService gRPC transport layer.
package api

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/omaigo88/inventory/pkg/repository"
	inventoryService "github.com/omaigo88/inventory/pkg/service"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
)

// Service is the business-logic dependency required by Server.
type Service interface {
	GetPart(ctx context.Context, rawUUID string) (repository.Part, error)
	ListParts(ctx context.Context, partType inventoryv1.PartType, rawUUIDs []string) ([]repository.Part, error)
}

// Server implements inventoryv1.InventoryServiceServer.
type Server struct {
	inventoryv1.UnimplementedInventoryServiceServer
	svc Service
}

// NewServer creates a new Server backed by the given Service.
func NewServer(svc Service) *Server {
	return &Server{svc: svc}
}

// toProto converts a repository.Part to inventoryv1.Part.
func toProto(p repository.Part) *inventoryv1.Part {
	return &inventoryv1.Part{
		Uuid:          p.UUID,
		Name:          p.Name,
		Description:   p.Description,
		Price:         p.Price,
		PartType:      p.PartType,
		StockQuantity: p.StockQuantity,
		CreatedAt:     p.CreatedAt,
	}
}

// toStatusErr maps a Service sentinel error to a gRPC status error.
func toStatusErr(err error) error {
	switch {
	case errors.Is(err, inventoryService.ErrEmptyUUID), errors.Is(err, inventoryService.ErrInvalidUUID):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, inventoryService.ErrPartNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// GetPart returns a part by UUID.
func (s *Server) GetPart(ctx context.Context, req *inventoryv1.GetPartRequest) (*inventoryv1.GetPartResponse, error) {
	part, err := s.svc.GetPart(ctx, req.GetUuid())
	if err != nil {
		return nil, toStatusErr(err)
	}

	return &inventoryv1.GetPartResponse{
		Part: toProto(part),
	}, nil
}

// ListParts returns a list of parts with optional filtering by type or UUIDs.
func (s *Server) ListParts(ctx context.Context, req *inventoryv1.ListPartsRequest) (*inventoryv1.ListPartsResponse, error) {
	parts, err := s.svc.ListParts(ctx, req.GetPartType(), req.GetUuids())
	if err != nil {
		return nil, toStatusErr(err)
	}

	protoParts := make([]*inventoryv1.Part, 0, len(parts))
	for _, part := range parts {
		protoParts = append(protoParts, toProto(part))
	}

	return &inventoryv1.ListPartsResponse{Parts: protoParts}, nil
}
