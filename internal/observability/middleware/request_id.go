package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

// RequestIDContextKey is a context key for storing request IDs.
type RequestIDContextKey struct{}

// getRequestID reads request ID from X-Request-ID header or context, generates if missing.
func getRequestID(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	if id, ok := r.Context().Value(RequestIDContextKey{}).(string); ok && id != "" {
		return id
	}
	return uuid.New().String()
}

// RequestIDGeneration reads request ID from client header or context, generates if missing,
// and stores it in request context for downstream handlers.
func RequestIDGeneration(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := getRequestID(r)

		// Store in request context for downstream middlewares
		ctx := context.WithValue(r.Context(), RequestIDContextKey{}, requestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDPropagation propagates the request ID to external consumers.
// Sets the X-Request-ID response header for client correlation and adds the ID
// to log attributes.
func RequestIDPropagation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestID, ok := r.Context().Value(RequestIDContextKey{}).(string); ok && requestID != "" {
			// Propagate to client via response header
			// Set early to ensure it's present during recovery scenarios
			w.Header().Set("X-Request-ID", requestID)

			// Propagate to logs
			SetLogAttrs(r.Context(), slog.String("request_id", requestID))
		}

		next.ServeHTTP(w, r)
	})
}
