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

	"github.com/omaigo88/order/internal/app"
)

func main() {
	ctx := context.Background()

	container, err := app.New(ctx)
	if err != nil {
		slog.Error("failed to build container", "error", err)
		os.Exit(1)
	}
	defer container.Close()

	httpServer := &http.Server{
		Addr:              ":8080",
		Handler:           container.Server,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	consumerDone := make(chan struct{})

	go func() {
		defer close(consumerDone)

		slog.Info("starting ShipAssembled consumer")

		if consumeErr := container.RunConsumer(ctx); consumeErr != nil {
			slog.Error("ShipAssembled consumer stopped", "error", consumeErr)
			stop() // take the HTTP server down too rather than run half a service
		}
	}()
	defer func() { <-consumerDone }()

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
