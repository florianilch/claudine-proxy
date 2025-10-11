package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/httplog/v3"
)

// Logging logs HTTP requests with method, path, status, and duration.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return httplog.RequestLogger(logger, &httplog.Options{
		Schema: httplog.SchemaECS.Concise(true),

		// Explicitly prevent logging headers/body to avoid leaking sensitive data
		LogRequestHeaders:  []string{"Content-Type", "Origin"}, // Default, but explicit
		LogResponseHeaders: []string{},                         // Explicit empty (default is empty, but be clear)
		LogRequestBody:     nil,                                // Never log request bodies (default, but explicit)
		LogResponseBody:    nil,                                // Never log response bodies (default, but explicit)

		RecoverPanics: false, // use dedicated middleware, panics are logged regardless
	})
}

// SetLogAttrs sets attributes on the request log.
func SetLogAttrs(ctx context.Context, attrs ...slog.Attr) {
	httplog.SetAttrs(ctx, attrs...)
}
