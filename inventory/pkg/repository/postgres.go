package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/omaigo88/inventory/pkg/domain"
)

// postgresRepository is a PostgreSQL-backed Repository implementation.
type postgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a Repository backed by the given pgx pool.
func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

func scanPartRecord(row interface{ Scan(...any) error }) (PartRecord, error) {
	var rec PartRecord

	err := row.Scan(
		&rec.UUID, &rec.Name, &rec.Description, &rec.Price,
		&rec.PartType, &rec.Properties, &rec.StockQuantity, &rec.Reserved, &rec.CreatedAt,
	)

	return rec, err
}

// GetPart returns a part by UUID.
func (r *postgresRepository) GetPart(ctx context.Context, id uuid.UUID) (domain.Part, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT uuid, name, description, price, part_type, properties, stock_quantity, reserved, created_at
		FROM parts
		WHERE uuid = $1
	`, id)

	rec, err := scanPartRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Part{}, ErrNotFound
	}

	if err != nil {
		return domain.Part{}, fmt.Errorf("get part: %w", err)
	}

	return RecordToDomain(rec)
}

// ListParts returns every part in the catalog.
func (r *postgresRepository) ListParts(ctx context.Context) ([]domain.Part, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT uuid, name, description, price, part_type, properties, stock_quantity, reserved, created_at
		FROM parts
	`)
	if err != nil {
		return nil, fmt.Errorf("list parts: %w", err)
	}
	defer rows.Close()

	var parts []domain.Part

	for rows.Next() {
		rec, scanErr := scanPartRecord(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list parts: %w", scanErr)
		}

		part, convErr := RecordToDomain(rec)
		if convErr != nil {
			return nil, fmt.Errorf("list parts: %w", convErr)
		}

		parts = append(parts, part)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list parts: %w", err)
	}

	return parts, nil
}

// Save persists part's Reserved count.
func (r *postgresRepository) Save(ctx context.Context, part domain.Part) error {
	tag, err := r.pool.Exec(ctx, `UPDATE parts SET reserved = $1 WHERE uuid = $2`, part.Reserved(), part.ID())
	if err != nil {
		return fmt.Errorf("save part: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
