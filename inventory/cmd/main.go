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

	inventoryService "github.com/omaigo88/inventory/pkg/service"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
)

const grpcAddress = ":50051"

func main() {
	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		slog.Error("не удалось создать listener", "error", err)
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
	inventoryv1.RegisterInventoryServiceServer(grpcServer, inventoryService.NewServer())

	// Включаем reflection для postman/grpcurl
	reflection.Register(grpcServer)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		slog.Info("получен сигнал завершения, останавливаем InventoryService")
		grpcServer.GracefulStop()
	}()

	slog.Info("запуск InventoryService", "адрес", grpcAddress)

	err = grpcServer.Serve(lis)
	if err != nil {
		slog.Error("ошибка запуска сервера", "error", err)
		os.Exit(1)
	}
}
