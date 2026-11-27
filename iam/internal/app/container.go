// Package app wires IAMService's dependency graph: telemetry (logs, traces),
// PostgreSQL user storage,
// Redis session storage, business logic, and the gRPC transport adapters.
package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/omaigo88/iam/pkg/api"
	"github.com/omaigo88/iam/pkg/repository"
	"github.com/omaigo88/iam/pkg/service"
	"github.com/omaigo88/shared/pkg/logger"
	"github.com/omaigo88/shared/pkg/telemetry"
	"github.com/omaigo88/shared/pkg/tracing"
)

const (
	defaultRedisAddress = "localhost:6379"
	defaultSessionTTL   = 24 * time.Hour
	serviceName         = "iam-service"
)

// Container holds IAMService's constructed dependencies.
type Container struct {
	pool      *pgxpool.Pool
	redis     *redis.Client
	telemetry []telemetry.ShutdownFunc

	// AuthServer implements authv1.AuthServiceServer.
	AuthServer *api.AuthServer
	// UserServer implements userv1.UserServiceServer.
	UserServer *api.UserServer
}

// New builds a Container connected to PostgreSQL (DB_URI) and Redis
// (REDIS_ADDRESS, default localhost:6379). Sessions live for IAM_SESSION_TTL
// (a Go duration, default 24h).
func New(ctx context.Context) (_ *Container, err error) {
	shutdowns, err := telemetry.Init(ctx, serviceName, logger.Init, tracing.InitTracer)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			telemetry.Flush(shutdowns...)
		}
	}()

	sessionTTL := defaultSessionTTL

	if raw := os.Getenv("IAM_SESSION_TTL"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("parse IAM_SESSION_TTL: %w", err)
		}

		sessionTTL = d
	}

	pool, err := pgxpool.New(ctx, os.Getenv("DB_URI"))
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	redisAddress := os.Getenv("REDIS_ADDRESS")
	if redisAddress == "" {
		redisAddress = defaultRedisAddress
	}

	redisClient := redis.NewClient(&redis.Options{Addr: redisAddress})

	if err := redisClient.Ping(ctx).Err(); err != nil {
		pool.Close()
		_ = redisClient.Close()

		return nil, fmt.Errorf("connect to redis: %w", err)
	}

	svc := service.NewService(
		repository.NewUserPostgresRepository(pool),
		repository.NewSessionRedisRepository(redisClient),
		sessionTTL,
	)

	return &Container{
		pool:       pool,
		redis:      redisClient,
		telemetry:  shutdowns,
		AuthServer: api.NewAuthServer(svc),
		UserServer: api.NewUserServer(svc),
	}, nil
}

// Close releases the Container's resources.
func (c *Container) Close() {
	_ = c.redis.Close()
	c.pool.Close()
	telemetry.Flush(c.telemetry...)
}
