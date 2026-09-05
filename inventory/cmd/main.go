package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	inventoryAPI "github.com/omaigo88/inventory/pkg/api"
	inventoryRepository "github.com/omaigo88/inventory/pkg/repository"
	inventoryService "github.com/omaigo88/inventory/pkg/service"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
)

const grpcAddress = ":50051"

func main() {
	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		slog.Error("failed to create listener", "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 15 * time.Minute,
			Time:              2 * time.Hour,
			Timeout:           20 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             1 * time.Minute,
			PermitWithoutStream: true,
		}),
	)
	repo := inventoryRepository.NewMemoryRepository()
	svc := inventoryService.NewService(repo)
	inventoryv1.RegisterInventoryServiceServer(grpcServer, inventoryAPI.NewServer(svc))

	// Enable reflection for postman/grpcurl
	reflection.Register(grpcServer)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		slog.Info("shutdown signal received, stopping InventoryService")
		grpcServer.GracefulStop()
	}()

	slog.Info("starting InventoryService", "address", grpcAddress)

	err = grpcServer.Serve(lis)
	if err != nil {
		slog.Error("error starting server", "error", err)
		os.Exit(1)
	}
}
