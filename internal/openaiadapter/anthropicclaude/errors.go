package anthropicclaude

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/florianilch/claudine-proxy/internal/openaiadapter/types"
)

// toChatCompletionError converts any error into OpenAI-compatible error format.
// Anthropic SDK returns different error shapes for streaming vs non-streaming requests,
// so we normalize both into a consistent ErrorResponse for SSE/JSON responses.
// Non-Anthropic errors (network, timeouts) are wrapped as generic server_error.
func toChatCompletionError(err error) *types.ErrorResponse {
	if err == nil {
		return nil
	}

	// Note: Anthropic error responses don't include 'code' or 'param' fields,
	// so these are always nil in the OpenAI-compatible response.

	// Non-streaming: *anthropic.Error provides structured error via RawJSON()
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		if errorResp, parseErr := parseErrorResponseJSON(apiErr.RawJSON()); parseErr == nil {
			return &types.ErrorResponse{
				Err: types.Error{
					Message: errorResp.Error.Message,
					Type:    mapAnthropicErrorType(errorResp.Error.Type),
				},
			}
		}
		// JSON parse failed, fallback to generic error wrapping
		return &types.ErrorResponse{
			Err: types.Error{
				Message: apiErr.Error(),
				Type:    "api_error",
			},
		}
	}

	// streamingErrorPrefix is the prefix used by the Anthropic SDK when wrapping streaming errors.
	const streamingErrorPrefix = "received error while streaming: "

	// Streaming: SDK embeds JSON in error string with known prefix
	if jsonStr, ok := strings.CutPrefix(err.Error(), streamingErrorPrefix); ok {
		if errorResp, parseErr := parseErrorResponseJSON(jsonStr); parseErr == nil {
			return &types.ErrorResponse{
				Err: types.Error{
					Message: errorResp.Error.Message,
					Type:    mapAnthropicErrorType(errorResp.Error.Type),
				},
			}
		}
	}

	// Fallback: wrap non-Anthropic errors (network, timeouts, etc.) as generic server_error
	return &types.ErrorResponse{
		Err: types.Error{
			Message: err.Error(),
			Type:    "server_error",
		},
	}
}

// parseErrorResponseJSON parses Anthropic error JSON into structured ErrorResponse.
// Shared by both non-streaming (RawJSON) and streaming (error string) error paths.
func parseErrorResponseJSON(jsonStr string) (*anthropic.ErrorResponse, error) {
	var errorResp anthropic.ErrorResponse
	if err := json.Unmarshal([]byte(jsonStr), &errorResp); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic error JSON: %w", err)
	}
	return &errorResp, nil
}

// mapAnthropicErrorType translates Anthropic error taxonomy to OpenAI-compatible error types.
func mapAnthropicErrorType(anthropicType string) string {
	switch anthropicType {
	case "overloaded_error":
		return "server_error"
	case "rate_limit_error":
		return "rate_limit_error"
	case "invalid_request_error":
		return "invalid_request_error"
	case "request_too_large":
		return "invalid_request_error"
	case "authentication_error":
		return "authentication_error"
	case "permission_error":
		return "permission_denied"
	case "not_found_error":
		return "invalid_request_error"
	case "timeout_error":
		return "server_error"
	case "api_error":
		return "api_error"
	case "billing_error":
		return "insufficient_quota"
	default:
		// Unknown error types default to api_error for safe handling
		return "api_error"
	}
}
