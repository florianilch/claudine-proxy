package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"

	"localhost/claude-proxy/internal/proxy"
)

// App orchestrates the lifecycle of the proxy server and related services.
type App struct {
	proxy *proxy.Proxy
}

// New creates a new App instance.
func New() (*App, error) {
	proxyServer, err := proxy.New("https://api.anthropic.com/v1")
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy: %w", err)
	}

	return &App{
		proxy: proxyServer,
	}, nil
}

// Start starts all services and blocks until shutdown is triggered.
// Uses errgroup for runtime error monitoring and shutdown function collection for coordinated cleanup.
func (a *App) Start(ctx context.Context) error {
	g, gCtx := errgroup.WithContext(ctx)

	var shutdownFuncs []func(context.Context) error

	// Startup phase: Start services
	slog.InfoContext(gCtx, "starting proxy server")
	proxyErrCh, err := a.proxy.Start(gCtx, "127.0.0.1:4000")
	if err != nil {
		return fmt.Errorf("proxy startup failed: %w", err)
	}
	shutdownFuncs = append(shutdownFuncs, a.proxy.Shutdown)

	// Monitor runtime errors - errgroup cancels context on first error
	g.Go(func() error {
		select {
		case err := <-proxyErrCh:
			if err != nil {
				slog.ErrorContext(gCtx, "proxy runtime error", "error", err)
				return fmt.Errorf("proxy: %w", err)
			}
			return nil
		case <-gCtx.Done():
			return nil
		}
	})

	runtimeErr := g.Wait()

	slog.InfoContext(gCtx, "shutting down services")

	// Shutdown phase: Stop all services
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var errs []error
	if runtimeErr != nil {
		errs = append(errs, fmt.Errorf("runtime: %w", runtimeErr))
	}

	for i := len(shutdownFuncs) - 1; i >= 0; i-- {
		if err := shutdownFuncs[i](shutdownCtx); err != nil {
			slog.ErrorContext(shutdownCtx, "service shutdown failed", "error", err)
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	slog.Info("application stopped")
	return nil
}
