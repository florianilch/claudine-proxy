package anthropicclaude

import (
	"fmt"
	"strconv"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/florianilch/claudine-proxy/internal/openaiadapter"
)

// buildGenerationParams builds Anthropic generation configuration from OpenAI request.
// Handles model, sampling parameters, tools, metadata, and all generation settings.
func buildGenerationParams(
	clientReq openaiadapter.CreateChatCompletionRequest,
) (anthropic.MessageNewParams, error) {
	params := anthropic.MessageNewParams{
		Model: anthropic.Model(clientReq.Model),
	}

	// MaxTokens is required in Anthropic API
	if clientReq.MaxCompletionTokens != nil {
		params.MaxTokens = int64(*clientReq.MaxCompletionTokens)
		//lint:ignore SA1019 Support for deprecated max_tokens field required for backward compatibility
	} else if clientReq.MaxTokens != nil { //nolint:staticcheck // Support deprecated max_tokens for backward compatibility
		params.MaxTokens = int64(*clientReq.MaxTokens) //nolint:staticcheck // Support deprecated max_tokens for backward compatibility
	} else {
		// Default to 8K tokens (reasonable for most use cases)
		// Users needing more can specify explicitly via max_completion_tokens
		params.MaxTokens = 8192
	}

	// Sampling parameters
	// Convert via string to avoid float32->float64 precision issues (0.7 -> "0.7" -> 0.7)
	if clientReq.Temperature != nil {
		temp, err := strconv.ParseFloat(fmt.Sprintf("%v", *clientReq.Temperature), 64)
		if err != nil {
			return params, fmt.Errorf("invalid temperature value: %w", err)
		}
		params.Temperature = anthropic.Float(temp)
	}
	if clientReq.TopP != nil {
		topP, err := strconv.ParseFloat(fmt.Sprintf("%v", *clientReq.TopP), 64)
		if err != nil {
			return params, fmt.Errorf("invalid top_p value: %w", err)
		}
		params.TopP = anthropic.Float(topP)
	}

	// Stop sequences
	if clientReq.Stop != nil {
		if sequences, err := clientReq.Stop.AsStopConfiguration1(); err == nil {
			params.StopSequences = sequences
		} else if single, err := clientReq.Stop.AsStopConfiguration0(); err == nil && single != "" {
			params.StopSequences = []string{single}
		}
	}

	// Tools
	if clientReq.Tools != nil {
		tools, err := fromChatCompletionTools(*clientReq.Tools)
		if err != nil {
			return params, fmt.Errorf("transform tools: %w", err)
		}
		params.Tools = tools
	}

	// Tool choice
	if clientReq.ToolChoice != nil {
		toolChoice, err := fromToolChoiceOption(clientReq.ToolChoice)
		if err != nil {
			return params, fmt.Errorf("transform tool choice: %w", err)
		}
		params.ToolChoice = toolChoice
	}

	// OpenAI user tracking fields to Anthropic's Metadata.UserID
	// SafetyIdentifier takes precedence over deprecated User field
	if clientReq.SafetyIdentifier != nil {
		params.Metadata = anthropic.MetadataParam{
			UserID: anthropic.String(*clientReq.SafetyIdentifier),
		}
	} else if clientReq.User != nil {
		params.Metadata = anthropic.MetadataParam{
			UserID: anthropic.String(*clientReq.User),
		}
	}

	// Build thinking configuration from reasoning effort and extra_body overrides
	thinking, err := buildThinking(clientReq)
	if err != nil {
		return params, fmt.Errorf("build thinking config: %w", err)
	}
	params.Thinking = thinking

	// ServiceTier transformation: Map OpenAI tiers to Anthropic equivalents
	if clientReq.ServiceTier != nil {
		// Map "auto" and "default" to Anthropic's "auto"
		serviceTier := string(*clientReq.ServiceTier)
		if serviceTier == "auto" || serviceTier == "default" {
			params.ServiceTier = anthropic.MessageNewParamsServiceTierAuto
		}
		// Other tiers (flex/scale/priority) have no equivalent in Anthropic
	}

	// LogitBias transformation: OpenAI's LogitBias (*map[string]int) allows fine-grained
	// control over individual token probabilities. Anthropic API does not provide
	// equivalent token-level bias controls. This is a semantic incompatibility.

	// FrequencyPenalty/PresencePenalty transformation: OpenAI's penalty parameters control
	// token repetition and presence. Anthropic's SDK does not seem to have equivalent penalty
	// parameters - only temperature and top_p for sampling control.

	// N (candidate count) transformation: OpenAI's N parameter generates multiple independent
	// completions. Anthropic API does not support multiple candidates per request.
	// This is a semantic incompatibility.

	// Seed transformation: OpenAI's Seed enables deterministic outputs. Anthropic API
	// does not have equivalent seed-based determinism controls.

	// Logprobs transformation: OpenAI's Logprobs enables token probability logging.
	// Anthropic API does not provide token-level log probabilities in responses.

	// Audio transformation: OpenAI's Audio configuration (Voice, Format) for audio outputs.
	// Anthropic API does not support audio output generation.

	// Modalities transformation: OpenAI's Modalities (text/audio) controls output modalities.
	// Anthropic supports text-only responses currently.

	// ResponseFormat transformation: OpenAI's ResponseFormat (text/json_object/json_schema)
	// controls output structure. Anthropic does not have equivalent structured output controls.

	// PromptCacheKey transformation: OpenAI's PromptCacheKey is client-provided cache key.
	// Anthropic's prompt caching uses automatic cache control breakpoints via CacheControl
	// on specific content blocks, not client-provided keys.

	// Prediction transformation: OpenAI's Prediction for "Predicted Outputs" optimization.
	// Anthropic has no equivalent predicted outputs mechanism.

	// Store transformation: OpenAI's Store controls conversation storage for model improvement.
	// Anthropic does not have equivalent opt-in storage mechanism.

	// Verbosity transformation: OpenAI's Verbosity (low/medium/high) is not supported.
	// Would require system prompt manipulation which may conflict with existing instructions.

	// ParallelToolCalls transformation: OpenAI's ParallelToolCalls controls parallel vs
	// sequential tool execution. Anthropic's DisableParallelToolUse in tool choice provides
	// equivalent control.
	if clientReq.ParallelToolCalls != nil && !*clientReq.ParallelToolCalls {
		// If parallel tool calls are disabled, set it in tool choice
		// This requires modifying the tool choice if it was set
		if tc := params.ToolChoice.OfAuto; tc != nil {
			tc.DisableParallelToolUse = anthropic.Bool(true)
		} else if tc := params.ToolChoice.OfAny; tc != nil {
			tc.DisableParallelToolUse = anthropic.Bool(true)
		} else {
			// No tool choice set, default to "auto" with parallel disabled
			params.ToolChoice.OfAuto = &anthropic.ToolChoiceAutoParam{
				DisableParallelToolUse: anthropic.Bool(true),
			}
		}
	}

	// WebSearchOptions transformation: OpenAI's WebSearchOptions enables web search via client
	// request parameter. Anthropic supports web search via server tools (WebSearchTool20250305)
	// that return ServerToolUseBlock and WebSearchResultBlock content types in responses.
	// These server-side tool blocks cannot be mapped to OpenAI's response format, which only
	// supports client-side tool calls. In multi-turn conversations, when the client sends
	// previous assistant messages back in history, we cannot reconstruct the original
	// ServerToolUseBlock/WebSearchResultBlock content - breaking conversation continuity.
	// This is an architectural incompatibility between server-side tools (Anthropic) and
	// client-side only tools (OpenAI).

	return params, nil
}
