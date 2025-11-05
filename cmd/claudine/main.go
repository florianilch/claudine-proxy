package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/florianilch/claudine-proxy/cmd/claudine/commands"
)

func main() {
	// Enable graceful shutdown via OS signals; context cancellation propagates to all commands.
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt,    // SIGINT: Ctrl+C (cross-platform)
		syscall.SIGTERM, // SIGTERM: Docker/k8s termination (Unix-only)
	)
	defer stop()

	if err := commands.Execute(ctx, os.Args); err != nil {
		slog.ErrorContext(ctx, "Application failed", "error", err)
		os.Exit(1)
	}
}
