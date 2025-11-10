package anthropicclaude

import (
	"github.com/anthropics/anthropic-sdk-go"

	"github.com/florianilch/claudine-proxy/internal/openaiadapter/types"
)

// toCompletionUsage converts Anthropic usage metadata to OpenAI CompletionUsage format.
// Transforms token counts including cached tokens from Anthropic's prompt caching.
func toCompletionUsage(usage anthropic.Usage) *types.CompletionUsage {
	completionUsage := &types.CompletionUsage{
		PromptTokens:     int(usage.InputTokens),
		CompletionTokens: int(usage.OutputTokens),
		TotalTokens:      int(usage.InputTokens + usage.OutputTokens),
	}

	// Anthropic's CacheReadInputTokens maps directly to OpenAI's cached_tokens
	if usage.CacheReadInputTokens > 0 {
		completionUsage.PromptTokensDetails = &struct {
			AudioTokens  *int `json:"audio_tokens,omitempty"`
			CachedTokens *int `json:"cached_tokens,omitempty"`
		}{
			CachedTokens: func() *int { v := int(usage.CacheReadInputTokens); return &v }(),
		}
	}

	// AudioTokens transformation: OpenAI's audio_tokens in prompt_tokens_details tracks
	// audio input tokens separately. Anthropic API does not provide separate
	// audio token counts in usage metadata - all tokens are aggregated.

	// CompletionTokensDetails transformation: OpenAI tracks reasoning_tokens separately
	// for extended thinking. Anthropic's thinking content is included in output_tokens
	// without separate breakdown. While ThinkingBlock content exists in responses,
	// usage metadata doesn't distinguish thinking tokens from regular output tokens.

	// AcceptedPredictionTokens/RejectedPredictionTokens transformation: OpenAI's
	// prediction token tracking for "Predicted Outputs" feature. Anthropic API
	// has no equivalent predicted outputs mechanism.

	return completionUsage
}
