package repository

import (
	"context"
	"errors"
	"fmt"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// postgresRepository is a PostgreSQL-backed Repository implementation.
type postgresRepository struct {
	pool   *pgxpool.Pool
	getter *trmpgx.CtxGetter
}

// NewPostgresRepository creates a Repository backed by the given pgx pool.
// Queries run inside an active transaction manager transaction (if one is
// present on ctx) or directly against the pool otherwise.
func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{
		pool:   pool,
		getter: trmpgx.DefaultCtxGetter,
	}
}

// Save upserts an order.
func (r *postgresRepository) Save(ctx context.Context, order Order) error {
	conn := r.getter.DefaultTrOrDB(ctx, r.pool)

	_, err := conn.Exec(
		ctx, `
		INSERT INTO orders (
			order_uuid, hull_uuid, engine_uuid, shield_uuid, weapon_uuid,
			total_price, transaction_uuid, payment_method, status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (order_uuid) DO UPDATE SET
			hull_uuid        = EXCLUDED.hull_uuid,
			engine_uuid      = EXCLUDED.engine_uuid,
			shield_uuid      = EXCLUDED.shield_uuid,
			weapon_uuid      = EXCLUDED.weapon_uuid,
			total_price      = EXCLUDED.total_price,
			transaction_uuid = EXCLUDED.transaction_uuid,
			payment_method   = EXCLUDED.payment_method,
			status           = EXCLUDED.status
	`,
		order.OrderUUID, order.HullUUID, order.EngineUUID, order.ShieldUUID, order.WeaponUUID,
		order.TotalPrice, order.TransactionUUID, order.PaymentMethod, order.Status, order.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save order: %w", err)
	}

	return nil
}

// Get returns an order by UUID.
func (r *postgresRepository) Get(ctx context.Context, id uuid.UUID) (Order, error) {
	conn := r.getter.DefaultTrOrDB(ctx, r.pool)

	var order Order

	err := conn.QueryRow(ctx, `
		SELECT order_uuid, hull_uuid, engine_uuid, shield_uuid, weapon_uuid,
		       total_price, transaction_uuid, payment_method, status, created_at
		FROM orders
		WHERE order_uuid = $1
	`, id).Scan(
		&order.OrderUUID, &order.HullUUID, &order.EngineUUID, &order.ShieldUUID, &order.WeaponUUID,
		&order.TotalPrice, &order.TransactionUUID, &order.PaymentMethod, &order.Status, &order.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrNotFound
	}

	if err != nil {
		return Order{}, fmt.Errorf("get order: %w", err)
	}

	return order, nil
}
