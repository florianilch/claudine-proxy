package proxy

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

// ErrorResponse represents a JSON error response.
type ErrorResponse struct {
	Error string `json:"error"`
}

// writeJSON writes a JSON response with the given status code.
// Logs encoding failures internally using the provided context.
func writeJSON(ctx context.Context, w http.ResponseWriter, data any, status int) {
	w.Header().Set("Content-Type", "application/json")
	// Headers and status are written before encoding to avoid buffering.
	// If encoding fails, the client may receive a partial response.
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.ErrorContext(ctx, "failed to encode JSON response", "error", err)
	}
}

// writeJSONError writes a JSON error response with the given status code.
// Similar to http.Error but returns JSON instead of plain text.
func writeJSONError(ctx context.Context, w http.ResponseWriter, message string, status int) {
	writeJSON(ctx, w, ErrorResponse{Error: message}, status)
}
