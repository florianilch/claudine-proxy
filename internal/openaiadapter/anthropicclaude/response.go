package anthropicclaude

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/florianilch/claudine-proxy/internal/openaiadapter/types"
)

// toFinishReason maps Anthropic stop reasons to OpenAI non-streaming finish reasons.
//
// Refusal transformation: OpenAI separates refusal text via a Refusal field, while
// Anthropic embeds refusals in content with stop_reason="refusal". We preserve
// Anthropic's approach by returning refusals as content with finish_reason="content_filter",
// ensuring consistency between streaming/non-streaming and correct round-trip handling
// when conversation history includes past refusals.
func toFinishReason(stopReason anthropic.StopReason) types.CreateChatCompletionResponseChoiceFinishReason {
	switch stopReason {
	case anthropic.StopReasonEndTurn:
		return types.CreateChatCompletionResponseChoiceFinishReasonStop
	case anthropic.StopReasonMaxTokens:
		return types.CreateChatCompletionResponseChoiceFinishReasonLength
	case anthropic.StopReasonStopSequence:
		return types.CreateChatCompletionResponseChoiceFinishReasonStop
	case anthropic.StopReasonToolUse:
		return types.CreateChatCompletionResponseChoiceFinishReasonToolCalls
	case anthropic.StopReasonRefusal:
		return types.CreateChatCompletionResponseChoiceFinishReasonContentFilter
	default:
		// PauseTurn transformation: Anthropic's "pause_turn" allows resuming long-running
		// turns in subsequent requests. OpenAI has no equivalent pause/resume mechanism
		// for long conversations. Map to "stop" as the closest semantic match.
		return types.CreateChatCompletionResponseChoiceFinishReasonStop
	}
}

// toFinishReasonStreaming maps Anthropic stop reasons to OpenAI streaming finish reasons.
// Mappings are identical to toFinishReason but return the streaming-specific type.
func toFinishReasonStreaming(stopReason anthropic.StopReason) types.CreateChatCompletionStreamResponseChoiceFinishReason {
	switch stopReason {
	case anthropic.StopReasonEndTurn:
		return types.CreateChatCompletionStreamResponseChoiceFinishReasonStop
	case anthropic.StopReasonMaxTokens:
		return types.CreateChatCompletionStreamResponseChoiceFinishReasonLength
	case anthropic.StopReasonStopSequence:
		return types.CreateChatCompletionStreamResponseChoiceFinishReasonStop
	case anthropic.StopReasonToolUse:
		return types.CreateChatCompletionStreamResponseChoiceFinishReasonToolCalls
	case anthropic.StopReasonRefusal:
		return types.CreateChatCompletionStreamResponseChoiceFinishReasonContentFilter
	default:
		// PauseTurn map to "stop" (see toFinishReason)
		return types.CreateChatCompletionStreamResponseChoiceFinishReasonStop
	}
}

// newResponseID generates an OpenAI-compatible response ID (chatcmpl-<token>).
// Used as fallback when Anthropic doesn't provide an ID in the response.
func newResponseID() string {
	b := make([]byte, 24) // 24 bytes yields 32 URL-safe base64 characters
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	// Use RawURLEncoding to avoid '+', '/' and trailing '='
	token := base64.RawURLEncoding.EncodeToString(b)
	return "chatcmpl-" + token
}
