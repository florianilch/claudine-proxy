package proxy

import (
	_ "embed"
	"log/slog"
	"net/http"
)

//go:embed models.json
var modelsJSON []byte

// modelsHandler returns a static list of available Anthropic models.
// The upstream /v1/models endpoint doesn't support OAuth authentication,
// so we serve a cached response to enable model selection in clients.
//
// The response uses a merged format compatible with both Anthropic and OpenAI
// clients, combining fields from both API specifications. This approach assumes
// that most clients ignore unknown fields.
func modelsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(modelsJSON); err != nil {
			slog.ErrorContext(r.Context(), "failed to write response", "error", err)
		}
	}
}
