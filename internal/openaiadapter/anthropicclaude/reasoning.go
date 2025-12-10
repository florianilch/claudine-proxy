package anthropicclaude

import (
	"fmt"
	"strconv"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/florianilch/claudine-proxy/internal/openaiadapter"
)

// buildThinking builds Anthropic's thinking configuration from OpenAI's reasoning effort.
// Maps OpenAI's effort levels (low/medium/high) to Anthropic's explicit token budgets.
// Also handles extra_body overrides for advanced users who want direct Anthropic config.
//
// Mapping: low ≈ 1,024 tokens, medium ≈ 8,192 tokens, high ≈ 24,576 tokens
//
// Override mechanism: Users can specify exact config via extra_body:
//
//	extra_body: {
//	    "thinking": {
//	        "type": "enabled",
//	        "budget_tokens": 16000
//	    }
//	}
func buildThinking(clientReq openaiadapter.CreateChatCompletionRequest) (anthropic.ThinkingConfigParamUnion, error) {
	var thinking anthropic.ThinkingConfigParamUnion

	if clientReq.ReasoningEffort != nil {
		switch *clientReq.ReasoningEffort {
		case "low":
			thinking = anthropic.ThinkingConfigParamOfEnabled(1024)
		case "medium":
			thinking = anthropic.ThinkingConfigParamOfEnabled(8192)
		case "high":
			thinking = anthropic.ThinkingConfigParamOfEnabled(24576)
		default:
			// Unknown reasoning_effort values are ignored; thinking remains unset
		}
	}

	// ExtraBody override: allows direct Anthropic configuration to supersede reasoning_effort mapping.
	// If budget_tokens is omitted, falls back to reasoning_effort's budget (if set).
	if clientReq.ExtraBody != nil {
		if thinkingConfig, ok := (*clientReq.ExtraBody)["thinking"].(map[string]any); ok {
			if typeVal, ok := thinkingConfig["type"].(string); ok {
				switch typeVal {
				case "enabled":
					var budgetTokens int64

					switch v := thinkingConfig["budget_tokens"].(type) {
					case float64:
						budgetTokens = int64(v)
					case string:
						parsed, err := strconv.ParseInt(v, 10, 64)
						if err != nil {
							return thinking, fmt.Errorf("invalid budget_tokens: must be a valid integer")
						}
						budgetTokens = parsed
					}

					if budgetTokens > 0 {
						thinking = anthropic.ThinkingConfigParamOfEnabled(budgetTokens)
					} else {
						// Require at least one budget source (reasoning_effort or budget_tokens)
						if thinking.GetType() == nil {
							return thinking, fmt.Errorf("extra_body.thinking.type is 'enabled' but budget_tokens not specified and no reasoning_effort set")
						}
					}
				case "disabled":
					thinking = anthropic.ThinkingConfigParamUnion{
						OfDisabled: &anthropic.ThinkingConfigDisabledParam{},
					}
				default:
					// Unknown thinking.type values are ignored
				}
			}
		}
	}

	return thinking, nil
}
