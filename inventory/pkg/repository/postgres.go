package repository

import (
	"context"
	"errors"
	"fmt"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/omaigo88/inventory/pkg/domain"
)

const selectPartColumns = `
	SELECT uuid, name, description, price, part_type, properties, stock_quantity, reserved, created_at
	FROM parts`

// postgresRepository is a PostgreSQL-backed Repository implementation.
type postgresRepository struct {
	pool   *pgxpool.Pool
	getter *trmpgx.CtxGetter
}

// NewPostgresRepository creates a Repository backed by the given pgx pool.
// Queries run inside an active transaction manager transaction (if one is
// present on ctx) or directly against the pool otherwise.
func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool, getter: trmpgx.DefaultCtxGetter}
}

func scanPartRecord(row interface{ Scan(...any) error }) (PartRecord, error) {
	var rec PartRecord

	err := row.Scan(
		&rec.UUID, &rec.Name, &rec.Description, &rec.Price,
		&rec.PartType, &rec.Properties, &rec.StockQuantity, &rec.Reserved, &rec.CreatedAt,
	)

	return rec, err
}

func (r *postgresRepository) getPart(ctx context.Context, query string, id uuid.UUID) (domain.Part, error) {
	conn := r.getter.DefaultTrOrDB(ctx, r.pool)

	rec, err := scanPartRecord(conn.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Part{}, ErrNotFound
	}

	if err != nil {
		return domain.Part{}, fmt.Errorf("get part: %w", err)
	}

	return RecordToDomain(rec)
}

// GetPart returns a part by UUID.
func (r *postgresRepository) GetPart(ctx context.Context, id uuid.UUID) (domain.Part, error) {
	return r.getPart(ctx, selectPartColumns+` WHERE uuid = $1`, id)
}

// GetPartForUpdate returns a part by UUID and locks its row until the
// surrounding transaction ends.
func (r *postgresRepository) GetPartForUpdate(ctx context.Context, id uuid.UUID) (domain.Part, error) {
	return r.getPart(ctx, selectPartColumns+` WHERE uuid = $1 FOR UPDATE`, id)
}

// ListParts returns every part in the catalog.
func (r *postgresRepository) ListParts(ctx context.Context) ([]domain.Part, error) {
	conn := r.getter.DefaultTrOrDB(ctx, r.pool)

	rows, err := conn.Query(ctx, selectPartColumns)
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

// Save persists part's reserved and stock quantities.
func (r *postgresRepository) Save(ctx context.Context, part domain.Part) error {
	conn := r.getter.DefaultTrOrDB(ctx, r.pool)

	tag, err := conn.Exec(
		ctx,
		`UPDATE parts SET reserved = $1, stock_quantity = $2 WHERE uuid = $3`,
		part.Reserved(), part.StockQuantity(), part.ID(),
	)
	if err != nil {
		return fmt.Errorf("save part: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
