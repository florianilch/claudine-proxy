//go:build goexperiment.jsonv2

package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"golang.org/x/oauth2"

	"github.com/florianilch/claudine-proxy/internal/observability/middleware"
	"github.com/florianilch/claudine-proxy/internal/openaiadapter/anthropicclaude"
)

const (
	// defaultBaseURL is the production Anthropic API endpoint
	defaultBaseURL = "https://api.anthropic.com/v1"
)

// Proxy represents the forward proxy server
type Proxy struct {
	mux    *http.ServeMux
	server *http.Server
}

// Compile-time check that Proxy implements http.Handler
var _ http.Handler = (*Proxy)(nil)

// ReadinessChecker represents the application's health state for readiness checks.
type ReadinessChecker interface {
	IsReady() bool
}

// config holds internal proxy configuration applied via Options.
type config struct {
	baseURL   string
	transport http.RoundTripper
}

// Option configures the proxy
type Option func(*config)

// WithTransport sets a custom transport (timeouts, TLS, connection pooling).
func WithTransport(rt http.RoundTripper) Option {
	return func(c *config) {
		c.transport = rt
	}
}

// WithBaseURL overrides the default upstream URL.
func WithBaseURL(baseURL string) Option {
	return func(c *config) {
		c.baseURL = baseURL
	}
}

// DefaultTransport returns a new http.Transport configured for API requirements.
// Clones http.DefaultTransport and adds ResponseHeaderTimeout to prevent indefinite hangs.
// Returns a fresh instance on each call to prevent accidental mutation.
//
// Modify the returned transport for custom timeout or connection settings:
//
//	transport := proxy.DefaultTransport()
//	transport.ResponseHeaderTimeout = 60 * time.Second
//	proxy.New(ts, proxy.WithTransport(transport))
func DefaultTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 30 * time.Second // Outbound: Wait for Anthropic response headers (prevents indefinite hangs)
	return t
}

// New creates a forward proxy configured for Anthropic API.
func New(ts oauth2.TokenSource, health ReadinessChecker, opts ...Option) (*Proxy, error) {
	cfg := &config{
		baseURL:   defaultBaseURL,
		transport: DefaultTransport(),
	}

	for _, opt := range opts {
		opt(cfg)
	}

	upstream, err := url.Parse(cfg.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid upstream URL: %w", err)
	}

	// Compose transport chain (request execution order):
	// oauth2.Transport → ImpersonationTransport → cfg.transport
	transport := &oauth2.Transport{
		Source: ts,
		Base: &ImpersonationTransport{
			Base: cfg.transport,
		},
	}

	// Build reverse proxy for Anthropic API
	reverseProxyHandler := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = upstream.Scheme
			pr.Out.URL.Host = upstream.Host
			pr.Out.Host = upstream.Host
		},
		// FlushInterval: -1 disables automatic periodic flushing, flushing only when the backend flushes.
		// This eliminates buffering delays, critical for streaming responses (SSE) where clients
		// expect immediate data as soon as the upstream API sends it.
		FlushInterval: -1,
		Transport:     transport,
	}

	// OpenAI SDK compatibility handler
	createChatCompletionsHandler := &CreateChatCompletionsHandler{
		Adapter:   anthropicclaude.NewCreateChatCompletionAdapter(),
		Transport: transport,
	}

	logger := slog.Default()

	mux := http.NewServeMux()

	// Forward proxy to Anthropic Messages API
	mux.Handle("POST "+upstream.Path+"/messages", applyMiddlewares(reverseProxyHandler,
		middleware.Logging(logger),
		Recovery,
		middleware.TraceContextExtraction,
		middleware.RequestIDGeneration,
		RequestSizeLimit(33<<20), // Anthropic enforces 32MB
		middleware.RequestIDPropagation,
	))

	// OpenAI SDK compatibility layer
	mux.Handle("POST "+upstream.Path+"/chat/completions", applyMiddlewares(createChatCompletionsHandler,
		middleware.Logging(logger),
		Recovery,
		middleware.TraceContextExtraction,
		middleware.RequestIDGeneration,
		RequestSizeLimit(31<<20), // proxy handles error
		middleware.RequestIDPropagation,
	))

	// Shared static Models API endpoint for OpenAI and Anthropic
	mux.Handle("GET "+upstream.Path+"/models", applyMiddlewares(modelsHandler(),
		middleware.Logging(logger),
		Recovery,
		middleware.TraceContextExtraction,
		middleware.RequestIDGeneration,
		middleware.RequestIDPropagation,
	))

	// Health check endpoints
	mux.HandleFunc("GET /health/liveness", livenessHandler())
	mux.HandleFunc("GET /health/readiness", readinessHandler(health))

	return &Proxy{mux: mux}, nil
}

// ServeHTTP implements http.Handler interface
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mux.ServeHTTP(w, r)
}

// Start starts the HTTP server in the background and returns immediately.
// Returns a channel for runtime errors and a startup error if any.
//
// Startup errors (port in use, permission denied) are returned immediately.
// Runtime errors (network failures during operation) are sent to the error channel.
//
// The caller is responsible for calling Shutdown() to stop the server.
func (p *Proxy) Start(ctx context.Context, address string) (<-chan error, error) {
	// Startup phase: Create listener synchronously to catch port-in-use errors immediately
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	p.server = &http.Server{
		Handler:      p,
		ReadTimeout:  30 * time.Second, // Inbound: Read entire client request (DoS protection against slow clients)
		WriteTimeout: 15 * time.Minute, // Inbound: Write entire response to client (allows long SSE streams, still bounded)
		IdleTimeout:  90 * time.Second, // Inbound: Keep-alive wait for next request from client
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	errCh := make(chan error, 1)

	go func() {
		err := p.server.Serve(listener)
		// Only report error if not from graceful shutdown
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	return errCh, nil
}

// Shutdown performs graceful shutdown of the HTTP server.
// Returns error if shutdown fails or times out.
func (p *Proxy) Shutdown(ctx context.Context) error {
	if p.server == nil {
		return nil
	}

	if err := p.server.Shutdown(ctx); err != nil {
		// Graceful shutdown failed - force close
		_ = p.server.Close()
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	return nil
}
