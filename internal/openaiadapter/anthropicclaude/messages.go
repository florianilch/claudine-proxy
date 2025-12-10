package anthropicclaude

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/florianilch/claudine-proxy/internal/openaiadapter/types"
)

// transformedMessage represents a single OpenAI message after transformation to Anthropic format.
// It preserves the original OpenAI role and contains the transformed content as either
// a MessageParam (for user/assistant/tool messages) or TextBlockParam (for system/developer messages).
// This intermediate representation maintains message ordering while allowing the caller to
// handle Anthropic's separate System field appropriately.
type transformedMessage struct {
	Role    string // Original OpenAI role (system, developer, user, assistant, tool)
	Content any    // Either *anthropic.MessageParam OR *anthropic.TextBlockParam
}

// contentPartWithText is a generic constraint for OpenAI content part union types.
// Enables type-safe text extraction across different message role content formats.
type contentPartWithText interface {
	AsChatCompletionRequestMessageContentPartText() (types.ChatCompletionRequestMessageContentPartText, error)
}

// fromChatCompletionRequestMessages converts OpenAI messages to Anthropic format.
// Returns transformedMessage structs preserving conversation order, with system/developer messages
// as TextBlockParam and user/assistant/tool messages as MessageParam. The caller is responsible
// for separating system blocks into Anthropic's System field while maintaining message ordering.
func fromChatCompletionRequestMessages(
	messages []types.ChatCompletionRequestMessage,
) ([]transformedMessage, error) {
	transformed := make([]transformedMessage, 0, len(messages))

	for msgIndex, msg := range messages {
		role, err := msg.Discriminator()
		if err != nil {
			return nil, fmt.Errorf("get type of message %d: %w", msgIndex, err)
		}

		switch role {
		case string(types.System):
			sysMsg, err := msg.AsChatCompletionRequestSystemMessage()
			if err != nil {
				return nil, fmt.Errorf("extract system message %d: %w", msgIndex, err)
			}
			textBlock, err := fromChatCompletionRequestSystemMessage(sysMsg, msgIndex)
			if err != nil {
				return nil, err
			}
			if textBlock == nil {
				continue
			}
			transformed = append(transformed, transformedMessage{
				Role:    string(types.System),
				Content: textBlock,
			})

		case string(types.ChatCompletionRequestDeveloperMessageRoleDeveloper):
			devMsg, err := msg.AsChatCompletionRequestDeveloperMessage()
			if err != nil {
				return nil, fmt.Errorf("extract developer message %d: %w", msgIndex, err)
			}
			textBlock, err := fromChatCompletionRequestDeveloperMessage(devMsg, msgIndex)
			if err != nil {
				return nil, err
			}
			if textBlock == nil {
				continue
			}
			transformed = append(transformed, transformedMessage{
				Role:    string(types.ChatCompletionRequestDeveloperMessageRoleDeveloper),
				Content: textBlock,
			})

		case string(types.User):
			userMsg, err := msg.AsChatCompletionRequestUserMessage()
			if err != nil {
				return nil, fmt.Errorf("extract user message %d: %w", msgIndex, err)
			}
			msgParam, err := fromChatCompletionRequestUserMessage(userMsg, msgIndex)
			if err != nil {
				return nil, err
			}
			if msgParam == nil {
				continue
			}
			transformed = append(transformed, transformedMessage{
				Role:    string(types.User),
				Content: msgParam,
			})

		case string(types.ChatCompletionRequestAssistantMessageRoleAssistant):
			assistMsg, err := msg.AsChatCompletionRequestAssistantMessage()
			if err != nil {
				return nil, fmt.Errorf("extract assistant message %d: %w", msgIndex, err)
			}
			msgParam, err := fromChatCompletionRequestAssistantMessage(assistMsg, msgIndex)
			if err != nil {
				return nil, err
			}
			if msgParam == nil {
				continue
			}
			transformed = append(transformed, transformedMessage{
				Role:    string(types.ChatCompletionRequestAssistantMessageRoleAssistant),
				Content: msgParam,
			})

		case string(types.Tool):
			toolMsg, err := msg.AsChatCompletionRequestToolMessage()
			if err != nil {
				return nil, fmt.Errorf("extract tool message %d: %w", msgIndex, err)
			}
			msgParam, err := fromChatCompletionRequestToolMessage(toolMsg, msgIndex)
			if err != nil {
				return nil, err
			}
			if msgParam == nil {
				continue
			}
			transformed = append(transformed, transformedMessage{
				Role:    string(types.Tool),
				Content: msgParam,
			})

		case string(types.ChatCompletionRequestFunctionMessageRoleFunction):
			return nil, fmt.Errorf("function messages not supported (deprecated in OpenAI API, use tool messages instead)")

		default:
			return nil, fmt.Errorf("unknown message role %s at index %d", role, msgIndex)
		}
	}

	// Merge consecutive tool messages to satisfy Anthropic's strict role alternation requirement
	return mergeConsecutiveToolMessages(transformed), nil
}

// mergeConsecutiveToolMessages combines consecutive tool messages into single user messages.
// Required because Anthropic enforces strict role alternation (no consecutive user messages),
// and tool results are sent as user messages containing tool_result blocks.
func mergeConsecutiveToolMessages(messages []transformedMessage) []transformedMessage {
	var result []transformedMessage
	var accumulatedToolBlocks []anthropic.ContentBlockParamUnion

	flushToolBlocks := func() {
		if len(accumulatedToolBlocks) > 0 {
			mergedMsg := anthropic.NewUserMessage(accumulatedToolBlocks...)
			result = append(result, transformedMessage{
				Role:    string(types.Tool),
				Content: &mergedMsg,
			})
			accumulatedToolBlocks = nil
		}
	}

	for _, msg := range messages {
		if msg.Role == string(types.Tool) {
			// Accumulate tool blocks
			if msgParam, ok := msg.Content.(*anthropic.MessageParam); ok {
				accumulatedToolBlocks = append(accumulatedToolBlocks, msgParam.Content...)
			}
		} else {
			// Non-tool message: flush any accumulated tool blocks, then add this message
			flushToolBlocks()
			result = append(result, msg)
		}
	}

	flushToolBlocks()

	return result
}

// fromChatCompletionRequestSystemMessage converts an OpenAI system message to Anthropic TextBlockParam.
func fromChatCompletionRequestSystemMessage(msg types.ChatCompletionRequestSystemMessage, msgIndex int) (*anthropic.TextBlockParam, error) {
	var content any
	if textContent, err := msg.Content.AsChatCompletionRequestSystemMessageContent0(); err == nil {
		content = textContent
	} else if arrayContent, err := msg.Content.AsChatCompletionRequestSystemMessageContent1(); err == nil {
		content = arrayContent
	} else {
		return nil, fmt.Errorf("extract system message %d content format: %w", msgIndex, err)
	}

	text := textFromOpenAIContent(content)
	if text == "" {
		// Return nil to skip: empty system messages serve no purpose and might be rejected.
		return nil, nil
	}

	return &anthropic.TextBlockParam{Text: text}, nil
}

// fromChatCompletionRequestDeveloperMessage converts an OpenAI developer message to Anthropic TextBlockParam.
// OpenAI's developer role provides model instructions similar to system messages but with different semantic intent.
// Anthropic API only supports system prompts via the System field with no developer role equivalent.
// Treat developer messages the same as system messages as the closest semantic match.
func fromChatCompletionRequestDeveloperMessage(msg types.ChatCompletionRequestDeveloperMessage, msgIndex int) (*anthropic.TextBlockParam, error) {
	var content any
	if textContent, err := msg.Content.AsChatCompletionRequestDeveloperMessageContent0(); err == nil {
		content = textContent
	} else if arrayContent, err := msg.Content.AsChatCompletionRequestDeveloperMessageContent1(); err == nil {
		content = arrayContent
	} else {
		return nil, fmt.Errorf("extract developer message %d content format: %w", msgIndex, err)
	}

	text := textFromOpenAIContent(content)
	if text == "" {
		// Return nil to skip: same reasoning as system messages
		return nil, nil
	}

	return &anthropic.TextBlockParam{Text: text}, nil
}

// fromChatCompletionRequestUserMessage converts an OpenAI user message to Anthropic MessageParam.
func fromChatCompletionRequestUserMessage(msg types.ChatCompletionRequestUserMessage, msgIndex int) (*anthropic.MessageParam, error) {
	var content any
	if textContent, err := msg.Content.AsChatCompletionRequestUserMessageContent0(); err == nil {
		content = textContent
	} else if arrayContent, err := msg.Content.AsChatCompletionRequestUserMessageContent1(); err == nil {
		content = arrayContent
	} else {
		return nil, fmt.Errorf("extract user message %d content format: %w", msgIndex, err)
	}

	contentBlocks, err := fromContentParts(content)
	if err != nil {
		return nil, fmt.Errorf("transform user message %d content: %w", msgIndex, err)
	}

	if len(contentBlocks) == 0 {
		// Return nil to skip: Anthropic rejects messages with empty content arrays.
		// Can occur when content parts produce no blocks (e.g., unsupported audio).
		return nil, nil
	}

	msgParam := anthropic.NewUserMessage(contentBlocks...)
	return &msgParam, nil
}

// fromChatCompletionRequestAssistantMessage converts an OpenAI assistant message to Anthropic MessageParam.
func fromChatCompletionRequestAssistantMessage(msg types.ChatCompletionRequestAssistantMessage, msgIndex int) (*anthropic.MessageParam, error) {
	var allBlocks []anthropic.ContentBlockParamUnion

	if msg.Content != nil {
		var content any
		if textContent, err := msg.Content.AsChatCompletionRequestAssistantMessageContent0(); err == nil {
			content = textContent
		} else if arrayContent, err := msg.Content.AsChatCompletionRequestAssistantMessageContent1(); err == nil {
			content = arrayContent
		} else {
			return nil, fmt.Errorf("extract assistant message %d content format: %w", msgIndex, err)
		}

		contentBlocks, err := fromContentParts(content)
		if err != nil {
			return nil, fmt.Errorf("transform assistant message %d content: %w", msgIndex, err)
		}
		allBlocks = append(allBlocks, contentBlocks...)
	}

	// Handle top-level refusal
	// Refusal transformation: OpenAI's refusal field (top-level) indicates model refused
	// to answer due to safety policies. Anthropic communicates refusals through stop_reason
	// "refusal" rather than explicit content field. Since this is assistant message in history,
	// preserve as text content for conversation continuity.
	if msg.Refusal != nil && *msg.Refusal != "" {
		allBlocks = append(allBlocks, anthropic.NewTextBlock(*msg.Refusal))
	}

	// msg.Name ignored: Anthropic does not support message names
	// msg.Audio ignored: contains only ID reference, not audio data

	if msg.ToolCalls != nil {
		for toolCallIdx, toolCallItem := range *msg.ToolCalls {
			toolCallDiscriminator, err := toolCallItem.Discriminator()
			if err != nil {
				return nil, fmt.Errorf("get type of tool call for assistant message %d, tool call %d: %w", msgIndex, toolCallIdx, err)
			}

			switch toolCallDiscriminator {
			case string(types.ChatCompletionMessageToolCallTypeFunction):
				toolCall, err := toolCallItem.AsChatCompletionMessageToolCall()
				if err != nil {
					return nil, fmt.Errorf("extract function tool call for assistant message %d, tool call %d: %w", msgIndex, toolCallIdx, err)
				}

				// Anthropic expects structured input data, not JSON strings.
				// Handle empty string case: json.Unmarshal fails on "" with "unexpected end of JSON input"
				inputObj := make(map[string]any)
				if args := strings.TrimSpace(toolCall.Function.Arguments); args != "" {
					if err := json.Unmarshal([]byte(args), &inputObj); err != nil {
						return nil, fmt.Errorf("unmarshal tool call arguments for assistant message %d, tool call %d: %w", msgIndex, toolCallIdx, err)
					}
				}

				toolUseBlock := anthropic.NewToolUseBlock(
					toolCall.Id,
					inputObj,
					toolCall.Function.Name,
				)
				allBlocks = append(allBlocks, toolUseBlock)

			default:
				return nil, fmt.Errorf("unsupported tool call type %s in assistant message %d, tool call %d", toolCallDiscriminator, msgIndex, toolCallIdx)
			}
		}
	}

	if msg.FunctionCall != nil {
		return nil, fmt.Errorf("function_call field not supported (deprecated in OpenAI API, use tool_calls instead)")
	}

	if len(allBlocks) == 0 {
		return nil, nil
	}

	msgParam := anthropic.NewAssistantMessage(allBlocks...)
	return &msgParam, nil
}

// fromChatCompletionRequestToolMessage converts an OpenAI tool message to Anthropic MessageParam.
// Tool results must be in user messages according to Anthropic's alternating turn pattern.
// Note: tool_call_id validation is performed server-side by Anthropic's API.
func fromChatCompletionRequestToolMessage(msg types.ChatCompletionRequestToolMessage, msgIndex int) (*anthropic.MessageParam, error) {
	var content any
	if textContent, err := msg.Content.AsChatCompletionRequestToolMessageContent0(); err == nil {
		content = textContent
	} else if arrayContent, err := msg.Content.AsChatCompletionRequestToolMessageContent1(); err == nil {
		content = arrayContent
	} else {
		return nil, fmt.Errorf("extract tool message %d content format: %w", msgIndex, err)
	}

	// OpenAI schema currently only supports text in tool messages
	resultText := textFromOpenAIContent(content)

	// IMPORTANT: Unlike other message types, we NEVER skip tool messages, even if empty.
	// Tool results can legitimately be empty (void functions, delete operations, etc.),
	// and the tool_call_id is required to close the tool invocation loop with Anthropic.
	// Skipping would break the assistant's tool calling flow.

	// is_error field limitation: OpenAI spec has no standard field to indicate tool execution errors.
	// Anthropic supports is_error to distinguish successful vs failed tool executions, allowing
	// Claude to retry with corrections. Without OpenAI equivalent, we always set false.
	toolResultBlock := anthropic.NewToolResultBlock(msg.ToolCallId, resultText, false)

	msgParam := anthropic.NewUserMessage(toolResultBlock)
	return &msgParam, nil
}

// textFromOpenAIContent extracts and concatenates text from OpenAI content formats.
// Handles both simple strings and content part arrays across all message role types.
func textFromOpenAIContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []types.ChatCompletionRequestUserMessageContentPart:
		return textFromOpenAIContentParts(v)
	case []types.ChatCompletionRequestAssistantMessageContentPart:
		return textFromOpenAIContentParts(v)
	case []types.ChatCompletionRequestToolMessageContentPart:
		return textFromOpenAIContentParts(v)
	case []types.ChatCompletionRequestSystemMessageContentPart:
		return textFromOpenAIContentParts(v)
	case []types.ChatCompletionRequestDeveloperMessageContentPart:
		return textFromOpenAIContentParts(v)
	default:
		return ""
	}
}

// textFromOpenAIContentParts extracts and concatenates text from OpenAI content part slices.
// Uses generics to handle content part unions across different message role types.
func textFromOpenAIContentParts[T contentPartWithText](parts []T) string {
	var texts []string
	for _, part := range parts {
		if textPart, err := part.AsChatCompletionRequestMessageContentPartText(); err == nil {
			texts = append(texts, textPart.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// textFromAnthropicContentBlocks extracts and concatenates text from Anthropic content blocks.
// Used when converting Anthropic responses back to OpenAI format.
func textFromAnthropicContentBlocks(content []anthropic.ContentBlockUnion) string {
	var texts []string
	for _, block := range content {
		switch variant := block.AsAny().(type) {
		case anthropic.TextBlock:
			if variant.Text != "" {
				texts = append(texts, variant.Text)
			}
		}
	}
	return strings.Join(texts, "\n")
}
