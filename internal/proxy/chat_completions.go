package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/florianilch/claudine-proxy/internal/openaiadapter"
	"github.com/florianilch/claudine-proxy/internal/openaiadapter/anthropicclaude"
	"github.com/florianilch/claudine-proxy/internal/openaiadapter/types"
)

// CreateChatCompletionsHandler handles OpenAI-compatible chat completion requests.
type CreateChatCompletionsHandler struct {
	Adapter   *anthropicclaude.CreateChatCompletionAdapter
	Transport http.RoundTripper
}

// Compile-time check to ensure CreateChatCompletionsHandler implements http.Handler
var _ http.Handler = (*CreateChatCompletionsHandler)(nil)

// ServeHTTP implements http.Handler interface for streaming or non-streaming requests.
func (h *CreateChatCompletionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req types.CreateChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			slog.WarnContext(ctx, "request exceeds size limit", "limit_bytes", maxBytesErr.Limit)
			writeJSONError(ctx, w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
			return
		}
		slog.ErrorContext(ctx, "failed to decode request", "error", err)
		writeJSONError(ctx, w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Stream != nil && *req.Stream {
		h.streamResponse(ctx, w, req)
	} else {
		h.writeResponse(ctx, w, req)
	}
}

// writeResponse handles non-streaming chat completion requests.
func (h *CreateChatCompletionsHandler) writeResponse(
	ctx context.Context,
	w http.ResponseWriter,
	req types.CreateChatCompletionRequest,
) {
	if ctx.Err() != nil {
		return
	}
	response, err := h.Adapter.ProcessRequest(ctx, req, h.Transport)
	if err != nil {
		slog.ErrorContext(ctx, "request failed", "error", err)
		writeJSONError(ctx, w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	writeJSON(ctx, w, response, http.StatusOK)
}

// streamResponse streams chat completion chunks using SSE.
func (h *CreateChatCompletionsHandler) streamResponse(
	ctx context.Context,
	w http.ResponseWriter,
	req types.CreateChatCompletionRequest,
) {
	if ctx.Err() != nil {
		return
	}
	stream, err := h.Adapter.ProcessStreamingRequest(ctx, req, h.Transport)
	if err != nil {
		slog.ErrorContext(ctx, "streaming request failed", "error", err)
		writeJSONError(ctx, w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	sse, err := NewSSEWriter(w)
	if err != nil {
		slog.ErrorContext(ctx, "SSE not supported", "error", err)
		writeJSONError(ctx, w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	for chunk, err := range stream {
		// Check for client disconnect before processing chunk
		if ctx.Err() != nil {
			slog.DebugContext(ctx, "client disconnected during stream")
			return
		}

		if err != nil {
			slog.ErrorContext(ctx, "stream error", "error", err)

			var errorResponse *openaiadapter.ChatCompletionErrorResponse
			if !errors.As(err, &errorResponse) {
				slog.ErrorContext(ctx, "unexpected error type", "error", err)
				return
			}

			// OpenAI SDK recognizes {"error": {...}} format and stops reading immediately
			// https://github.com/openai/openai-go/blob/ae042a437e4ebef4dffe088bf01d087ac94feaf2/packages/ssestream/ssestream.go#L169-L173
			if writeErr := sse.WriteData(errorResponse); writeErr != nil {
				slog.ErrorContext(ctx, "failed to write error", "error", writeErr)
			}
			return
		}

		if err := sse.WriteData(chunk); err != nil {
			slog.ErrorContext(ctx, "failed to write chunk", "error", err)
			return
		}
	}

	// OpenAI streaming protocol requires [DONE] marker
	if err := sse.WriteRaw("[DONE]"); err != nil {
		slog.ErrorContext(ctx, "failed to write stream termination marker", "error", err)
	}
}
