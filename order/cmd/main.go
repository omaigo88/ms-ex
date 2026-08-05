package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	orderHandler "github.com/omaigo88/order/pkg/handler"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
	paymentv1 "github.com/omaigo88/shared/pkg/proto/payment/v1"
)

const (
	inventoryServiceAddress = "localhost:50051"
	paymentServiceAddress   = "localhost:50052"
)

var grpcKeepaliveParams = grpc.WithKeepaliveParams(keepalive.ClientParameters{
	Time:                30 * time.Second,
	Timeout:             10 * time.Second,
	PermitWithoutStream: true,
})

func main() {
	// Создать gRPC соединение с InventoryService
	inventoryConn, err := grpc.NewClient(inventoryServiceAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpcKeepaliveParams,
	)
	if err != nil {
		slog.Error("не удалось подключиться к InventoryService", "error", err)
		os.Exit(1)
	}
	defer inventoryConn.Close()

	// Создать gRPC соединение с PaymentService
	paymentConn, err := grpc.NewClient(paymentServiceAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpcKeepaliveParams,
	)
	if err != nil {
		slog.Error("не удалось подключиться к PaymentService", "error", err)
		os.Exit(1)
	}
	defer paymentConn.Close()

	// Создаём хранилище и обработчик
	store := orderHandler.NewOrderStore()
	h := orderHandler.NewHandler(
		inventoryv1.NewInventoryServiceClient(inventoryConn),
		paymentv1.NewPaymentServiceClient(paymentConn),
		store,
	)

	// Создать OpenAPI сервер
	orderServer, err := orderHandler.SetupServer(h)
	if err != nil {
		slog.Error("ошибка создания сервера OpenAPI", "error", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:              ":8080",
		Handler:           orderServer,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		slog.Info("получен сигнал завершения, останавливаем OrderService")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if shutdownErr := httpServer.Shutdown(shutdownCtx); shutdownErr != nil {
			slog.Error("ошибка при остановке HTTP сервера", "error", shutdownErr)
		}
	}()

	slog.Info("запуск OrderService", "port", 8080)

	err = httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("ошибка запуска сервера", "error", err)
		os.Exit(1)
	}
}
