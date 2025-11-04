package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"

	"github.com/florianilch/claudine-proxy/internal/proxy"
	anthropictokensource "github.com/florianilch/claudine-proxy/internal/tokensource"
	"github.com/florianilch/claudine-proxy/internal/tokenstore"
)

// App orchestrates the lifecycle of the proxy server and related services.
type App struct {
	proxy *proxy.Proxy
}

// New creates a new App instance.
func New() (*App, error) {
	// I/O deferred to first Token() call
	tokenSource, err := newTokenSource()
	if err != nil {
		return nil, fmt.Errorf("failed to create token source: %w", err)
	}

	proxyServer, err := proxy.New(tokenSource, "https://api.anthropic.com/v1")
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

// newTokenSource creates a PersistentTokenSource from application configuration.
// No I/O is performed - TokenSource creation is deferred to first Token() call.
func newTokenSource() (*PersistentTokenSource, error) {
	store, err := tokenstore.NewFileStore("./auth")
	if err != nil {
		return nil, fmt.Errorf("failed to create token store: %w", err)
	}

	factory := func(token string) oauth2.TokenSource {
		return anthropictokensource.NewTokenSource(token, anthropictokensource.Endpoint)
	}

	return NewPersistentTokenSource(factory, store)
}
