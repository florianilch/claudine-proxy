package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"localhost/claude-proxy/internal/app"
	"localhost/claude-proxy/internal/observability"
)

func main() {
	// Enable graceful shutdown via OS signals; context cancellation propagates to all commands.
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt,    // SIGINT: Ctrl+C (cross-platform)
		syscall.SIGTERM, // SIGTERM: Docker/k8s termination (Unix-only)
	)
	defer stop()

	if err := run(ctx); err != nil {
		slog.ErrorContext(ctx, "Application failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// Set up observability before creating app
	observability.Instrument()

	application, err := app.New()
	if err != nil {
		return fmt.Errorf("failed to create app: %w", err)
	}

	slog.InfoContext(ctx, "starting")

	if err := application.Start(ctx); err != nil {
		return fmt.Errorf("app failed to start: %w", err)
	}

	slog.InfoContext(ctx, "stopped gracefully")
	return nil
}
