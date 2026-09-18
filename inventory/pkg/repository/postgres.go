package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
)

// postgresRepository is a PostgreSQL-backed Repository implementation.
type postgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a Repository backed by the given pgx pool.
func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

// GetPart returns a part by UUID.
func (r *postgresRepository) GetPart(ctx context.Context, id uuid.UUID) (Part, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT uuid, name, description, price, part_type, stock_quantity, created_at
		FROM parts
		WHERE uuid = $1
	`, id)

	var (
		p         Part
		partUUID  uuid.UUID
		partType  int32
		createdAt time.Time
	)

	err := row.Scan(&partUUID, &p.Name, &p.Description, &p.Price, &partType, &p.StockQuantity, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Part{}, ErrNotFound
	}

	if err != nil {
		return Part{}, fmt.Errorf("get part: %w", err)
	}

	p.UUID = partUUID.String()
	p.PartType = inventoryv1.PartType(partType)
	p.CreatedAt = timestamppb.New(createdAt)

	return p, nil
}

// ListParts returns every part in the catalog.
func (r *postgresRepository) ListParts(ctx context.Context) ([]Part, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT uuid, name, description, price, part_type, stock_quantity, created_at
		FROM parts
	`)
	if err != nil {
		return nil, fmt.Errorf("list parts: %w", err)
	}
	defer rows.Close()

	var parts []Part

	for rows.Next() {
		var (
			p         Part
			partUUID  uuid.UUID
			partType  int32
			createdAt time.Time
		)

		if scanErr := rows.Scan(&partUUID, &p.Name, &p.Description, &p.Price, &partType, &p.StockQuantity, &createdAt); scanErr != nil {
			return nil, fmt.Errorf("list parts: %w", scanErr)
		}

		p.UUID = partUUID.String()
		p.PartType = inventoryv1.PartType(partType)
		p.CreatedAt = timestamppb.New(createdAt)
		parts = append(parts, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list parts: %w", err)
	}

	return parts, nil
}
