package service

import (
	"context"
	"sort"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
)

// Part represents a spaceship part
type Part struct {
	UUID          string
	Name          string
	Description   string
	Price         int64 // in kopecks
	PartType      inventoryv1.PartType
	StockQuantity int64
	CreatedAt     *timestamppb.Timestamp
}

// server implements the gRPC service
type server struct {
	inventoryv1.UnimplementedInventoryServiceServer
	parts map[uuid.UUID]Part
}

// NewServer creates a server pre-populated with seed data
func NewServer() *server {
	now := timestamppb.Now()

	return &server{
		parts: map[uuid.UUID]Part{
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"): {
				UUID:          "550e8400-e29b-41d4-a716-446655440001",
				Name:          "Aluminum Hull",
				Description:   "Lightweight hull for small ships",
				Price:         500000, // 5000₽
				PartType:      inventoryv1.PartType_PART_TYPE_HULL,
				StockQuantity: 10,
				CreatedAt:     now,
			},
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440002"): {
				UUID:          "550e8400-e29b-41d4-a716-446655440002",
				Name:          "Titanium Hull",
				Description:   "Durable hull for medium ships",
				Price:         1500000, // 15000₽
				PartType:      inventoryv1.PartType_PART_TYPE_HULL,
				StockQuantity: 5,
				CreatedAt:     now,
			},
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440003"): {
				UUID:          "550e8400-e29b-41d4-a716-446655440003",
				Name:          "Ion Engine C",
				Description:   "Basic class C ion engine",
				Price:         300000, // 3000₽
				PartType:      inventoryv1.PartType_PART_TYPE_ENGINE,
				StockQuantity: 8,
				CreatedAt:     now,
			},
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440004"): {
				UUID:          "550e8400-e29b-41d4-a716-446655440004",
				Name:          "Ion Engine B",
				Description:   "Improved class B ion engine",
				Price:         800000, // 8000₽
				PartType:      inventoryv1.PartType_PART_TYPE_ENGINE,
				StockQuantity: 3,
				CreatedAt:     now,
			},
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440005"): {
				UUID:          "550e8400-e29b-41d4-a716-446655440005",
				Name:          "Energy Shield",
				Description:   "Standard energy shield",
				Price:         400000, // 4000₽
				PartType:      inventoryv1.PartType_PART_TYPE_SHIELD,
				StockQuantity: 6,
				CreatedAt:     now,
			},
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440006"): {
				UUID:          "550e8400-e29b-41d4-a716-446655440006",
				Name:          "Laser Cannon",
				Description:   "Precision laser cannon",
				Price:         250000, // 2500₽
				PartType:      inventoryv1.PartType_PART_TYPE_WEAPON,
				StockQuantity: 7,
				CreatedAt:     now,
			},
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440007"): {
				UUID:          "550e8400-e29b-41d4-a716-446655440007",
				Name:          "Plasma Hull",
				Description:   "Experimental hull (out of stock)",
				Price:         2000000, // 20000₽
				PartType:      inventoryv1.PartType_PART_TYPE_HULL,
				StockQuantity: 0,
				CreatedAt:     now,
			},
		},
	}
}

// toProto converts a Part to inventoryv1.Part
func toProto(p Part) *inventoryv1.Part {
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

// GetPart returns a part by UUID
func (s *server) GetPart(
	ctx context.Context,
	req *inventoryv1.GetPartRequest,
) (*inventoryv1.GetPartResponse, error) {
	if req.GetUuid() == "" {
		return nil, status.Error(codes.InvalidArgument, "uuid must not be empty")
	}

	partUUID, err := uuid.Parse(req.GetUuid())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid uuid format: %s", req.GetUuid())
	}

	part, ok := s.parts[partUUID]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "part not found: %s", req.GetUuid())
	}

	return &inventoryv1.GetPartResponse{
		Part: toProto(part),
	}, nil
}

// ListParts returns a list of parts with optional filtering by type
func (s *server) ListParts(
	ctx context.Context,
	req *inventoryv1.ListPartsRequest,
) (*inventoryv1.ListPartsResponse, error) {
	if len(req.GetUuids()) > 0 {
		parts := make([]*inventoryv1.Part, 0, len(req.GetUuids()))

		for _, rawUUID := range req.GetUuids() {
			partUUID, err := uuid.Parse(rawUUID)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid uuid format: %s", rawUUID)
			}

			part, ok := s.parts[partUUID]
			if !ok {
				return nil, status.Errorf(codes.NotFound, "part not found: %s", rawUUID)
			}

			parts = append(parts, toProto(part))
		}

		return &inventoryv1.ListPartsResponse{Parts: parts}, nil
	}

	parts := make([]*inventoryv1.Part, 0, len(s.parts))

	for _, part := range s.parts {
		if req.GetPartType() != inventoryv1.PartType_PART_TYPE_UNSPECIFIED && part.PartType != req.GetPartType() {
			continue
		}

		parts = append(parts, toProto(part))
	}

	sort.Slice(parts, func(i, j int) bool {
		return parts[i].GetName() < parts[j].GetName()
	})

	return &inventoryv1.ListPartsResponse{Parts: parts}, nil
}
