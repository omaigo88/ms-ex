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
	// Create a gRPC connection to InventoryService
	inventoryConn, err := grpc.NewClient(
		inventoryServiceAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpcKeepaliveParams,
	)
	if err != nil {
		slog.Error("failed to connect to InventoryService", "error", err)
		os.Exit(1)
	}
	defer inventoryConn.Close()

	// Create a gRPC connection to PaymentService
	paymentConn, err := grpc.NewClient(
		paymentServiceAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpcKeepaliveParams,
	)
	if err != nil {
		slog.Error("failed to connect to PaymentService", "error", err)
		os.Exit(1)
	}
	defer paymentConn.Close()

	// Create the store and handler
	store := orderHandler.NewOrderStore()
	h := orderHandler.NewHandler(
		inventoryv1.NewInventoryServiceClient(inventoryConn),
		paymentv1.NewPaymentServiceClient(paymentConn),
		store,
	)

	// Create the OpenAPI server
	orderServer, err := orderHandler.SetupServer(h)
	if err != nil {
		slog.Error("failed to create OpenAPI server", "error", err)
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
		slog.Info("shutdown signal received, stopping OrderService")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if shutdownErr := httpServer.Shutdown(shutdownCtx); shutdownErr != nil {
			slog.Error("error shutting down HTTP server", "error", shutdownErr)
		}
	}()

	slog.Info("starting OrderService", "port", 8080)

	err = httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("error starting server", "error", err)
		os.Exit(1)
	}
}
