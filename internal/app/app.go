package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"

	"github.com/florianilch/claudine-proxy/internal/proxy"
	anthropictokensource "github.com/florianilch/claudine-proxy/internal/tokensource"
)

// App orchestrates the lifecycle of the proxy server and related services.
type App struct {
	cfg    *Config
	proxy  *proxy.Proxy
	health *Health
}

// New creates a new App instance.
func New(cfg *Config) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	health := NewHealth()

	// I/O deferred to first Token() call
	tokenSource, err := newTokenSource(cfg.Auth)
	if err != nil {
		return nil, fmt.Errorf("failed to create token source: %w", err)
	}

	proxyServer, err := proxy.New(tokenSource, health, proxy.WithBaseURL(cfg.Upstream.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy: %w", err)
	}

	return &App{
		cfg:    cfg,
		proxy:  proxyServer,
		health: health,
	}, nil
}

// Start starts all services and blocks until shutdown is triggered.
// Uses errgroup for runtime error monitoring and shutdown function collection for coordinated cleanup.
func (a *App) Start(ctx context.Context) error {
	g, gCtx := errgroup.WithContext(ctx)

	address := a.cfg.Server.Host + ":" + strconv.FormatUint(uint64(a.cfg.Server.Port), 10)
	var shutdownFuncs []func(context.Context) error

	// Startup phase: Start services
	slog.InfoContext(gCtx, "starting proxy server", "address", address)
	proxyErrCh, err := a.proxy.Start(gCtx, address)
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

	a.health.SetReady(true)
	slog.InfoContext(gCtx, "application ready", "address", address)

	runtimeErr := g.Wait()

	a.health.SetReady(false)
	slog.InfoContext(gCtx, "shutting down services")
	time.Sleep(a.cfg.Shutdown.Delay)

	// Shutdown phase: Stop all services
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.Shutdown.Timeout)
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
func newTokenSource(cfg AuthConfig) (*PersistentTokenSource, error) {
	store, err := cfg.NewTokenStore()
	if err != nil {
		return nil, fmt.Errorf("failed to create token store: %w", err)
	}

	var factory TokenSourceFactory

	switch cfg.Method {
	case AuthenticationMethodOAuth:
		factory = func(token string) oauth2.TokenSource {
			return anthropictokensource.NewTokenSource(token, anthropictokensource.Endpoint)
		}
	case AuthenticationMethodStatic:
		factory = func(token string) oauth2.TokenSource {
			return oauth2.StaticTokenSource(&oauth2.Token{
				AccessToken: token,
				TokenType:   "Bearer",
			})
		}
	default:
		return nil, fmt.Errorf("unsupported authentication method: %s", cfg.Method)
	}

	return NewPersistentTokenSource(factory, store)
}
