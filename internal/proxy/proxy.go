package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// Proxy represents the forward proxy server
type Proxy struct {
	mux    *http.ServeMux
	server *http.Server
}

// Compile-time check that Proxy implements http.Handler
var _ http.Handler = (*Proxy)(nil)

// New creates a forward proxy.
func New(baseURL string) (*Proxy, error) {
	upstream, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid upstream URL: %w", err)
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
	}

	mux := http.NewServeMux()

	// Forward proxy to Anthropic Messages API
	mux.Handle("POST "+upstream.Path+"/messages", reverseProxyHandler)

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
