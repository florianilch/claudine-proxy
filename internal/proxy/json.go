package proxy

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/florianilch/claudine-proxy/internal/openaiadapter"
)

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

// writeJSONOpenAIError writes an OpenAI-compatible error response with the appropriate HTTP status code.
// The status code is determined from the error type according to OpenAI API conventions.
func writeJSONOpenAIError(ctx context.Context, w http.ResponseWriter, errResp *openaiadapter.ErrorResponse) {
	// Map OpenAI error types to HTTP status codes
	var status int
	switch errResp.Err.Type {
	case "invalid_request_error":
		status = http.StatusBadRequest
	case "authentication_error":
		status = http.StatusUnauthorized
	case "permission_denied":
		status = http.StatusForbidden
	case "rate_limit_error":
		status = http.StatusTooManyRequests
	case "insufficient_quota":
		status = http.StatusTooManyRequests
	case "server_error":
		status = http.StatusInternalServerError
	case "api_error":
		status = http.StatusInternalServerError
	default:
		status = http.StatusInternalServerError
	}

	writeJSON(ctx, w, errResp, status)
}
