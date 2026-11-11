package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/omaigo88/assembly/internal/app"
)

func main() {
	container, err := app.New()
	if err != nil {
		slog.Error("failed to build container", "error", err)
		os.Exit(1)
	}
	defer container.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("starting AssemblyService")

	if err := container.Run(ctx); err != nil {
		slog.Error("AssemblyService stopped", "error", err)
		os.Exit(1)
	}

	slog.Info("shutdown signal received, AssemblyService stopped")
}
