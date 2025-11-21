package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/florianilch/claudine-proxy/internal/openaiadapter"
	"github.com/florianilch/claudine-proxy/internal/openaiadapter/anthropicclaude"
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

	var req openaiadapter.CreateChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			slog.WarnContext(ctx, "request exceeds size limit", "limit_bytes", maxBytesErr.Limit)
			writeJSONOpenAIError(ctx, w, &openaiadapter.ErrorResponse{
				Err: openaiadapter.Error{
					Message: http.StatusText(http.StatusRequestEntityTooLarge),
					Type:    "invalid_request_error",
				},
			})
			return
		}
		slog.ErrorContext(ctx, "failed to decode request", "error", err)
		writeJSONOpenAIError(ctx, w, &openaiadapter.ErrorResponse{
			Err: openaiadapter.Error{
				Message: http.StatusText(http.StatusBadRequest),
				Type:    "invalid_request_error",
			},
		})
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
	req openaiadapter.CreateChatCompletionRequest,
) {
	if ctx.Err() != nil {
		return
	}
	response, err := h.Adapter.ProcessRequest(ctx, req, h.Transport)
	if err != nil {
		slog.ErrorContext(ctx, "request failed", "error", err)

		var errResp *openaiadapter.ErrorResponse
		if errors.As(err, &errResp) {
			writeJSONOpenAIError(ctx, w, errResp)
			return
		}

		writeJSONOpenAIError(ctx, w, &openaiadapter.ErrorResponse{
			Err: openaiadapter.Error{
				Message: http.StatusText(http.StatusInternalServerError),
				Type:    "api_error",
			},
		})
		return
	}

	writeJSON(ctx, w, response, http.StatusOK)
}

// streamResponse streams chat completion chunks using SSE.
func (h *CreateChatCompletionsHandler) streamResponse(
	ctx context.Context,
	w http.ResponseWriter,
	req openaiadapter.CreateChatCompletionRequest,
) {
	if ctx.Err() != nil {
		return
	}
	stream, err := h.Adapter.ProcessStreamingRequest(ctx, req, h.Transport)
	if err != nil {
		slog.ErrorContext(ctx, "streaming request failed", "error", err)

		var errResp *openaiadapter.ErrorResponse
		if errors.As(err, &errResp) {
			writeJSONOpenAIError(ctx, w, errResp)
			return
		}

		writeJSONOpenAIError(ctx, w, &openaiadapter.ErrorResponse{
			Err: openaiadapter.Error{
				Message: http.StatusText(http.StatusInternalServerError),
				Type:    "api_error",
			},
		})
		return
	}

	sse, err := NewSSEWriter(w)
	if err != nil {
		slog.ErrorContext(ctx, "SSE not supported", "error", err)
		writeJSONOpenAIError(ctx, w, &openaiadapter.ErrorResponse{
			Err: openaiadapter.Error{
				Message: http.StatusText(http.StatusInternalServerError),
				Type:    "api_error",
			},
		})
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

			var errorResponse *openaiadapter.ErrorResponse
			if errors.As(err, &errorResponse) {
				// OpenAI SDK recognizes {"error": {...}} format and stops reading immediately
				// https://github.com/openai/openai-go/blob/ae042a437e4ebef4dffe088bf01d087ac94feaf2/packages/ssestream/ssestream.go#L169-L173
				if writeErr := sse.WriteEvent("error"); writeErr != nil {
					slog.ErrorContext(ctx, "failed to write error event type", "error", writeErr)
					return
				}
				if writeErr := sse.WriteData(errorResponse); writeErr != nil {
					slog.ErrorContext(ctx, "failed to write error", "error", writeErr)
				}
				return
			}

			// Fallback: wrap unexpected errors for client visibility
			slog.ErrorContext(ctx, "unexpected error type, wrapping in fallback", "error", err)
			fallbackErr := &openaiadapter.ErrorResponse{
				Err: openaiadapter.Error{
					Message: err.Error(),
					Type:    "api_error",
				},
			}
			if writeErr := sse.WriteEvent("error"); writeErr != nil {
				slog.ErrorContext(ctx, "failed to write fallback error event type", "error", writeErr)
				return
			}
			if writeErr := sse.WriteData(fallbackErr); writeErr != nil {
				slog.ErrorContext(ctx, "failed to write fallback error", "error", writeErr)
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
