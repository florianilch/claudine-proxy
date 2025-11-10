package anthropicclaude

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"

	"github.com/florianilch/claudine-proxy/internal/openaiadapter/types"
)

// fromChatCompletionTools transforms OpenAI tools array to Anthropic format.
func fromChatCompletionTools(
	tools []types.CreateChatCompletionRequest_Tools_Item,
) ([]anthropic.ToolUnionParam, error) {
	if len(tools) == 0 {
		return nil, nil
	}

	anthropicTools := make([]anthropic.ToolUnionParam, 0, len(tools))
	for i, toolItem := range tools {
		discriminator, err := toolItem.Discriminator()
		if err != nil {
			return nil, fmt.Errorf("get type of tool %d: %w", i, err)
		}

		switch discriminator {
		case string(types.Function):
			chatTool, err := toolItem.AsChatCompletionTool()
			if err != nil {
				return nil, fmt.Errorf("extract function tool %d: %w", i, err)
			}

			toolParam := anthropic.ToolParam{
				Name:        chatTool.Function.Name,
				InputSchema: anthropic.ToolInputSchemaParam{},
			}

			if chatTool.Function.Description != nil {
				toolParam.Description = anthropic.String(*chatTool.Function.Description)
			}

			// Transform schema format: OpenAI uses flat JSON Schema object, Anthropic separates
			// properties/required into distinct fields with remaining fields in ExtraFields.
			if chatTool.Function.Parameters != nil {
				params := *chatTool.Function.Parameters

				if props, ok := params["properties"]; ok {
					toolParam.InputSchema.Properties = props
				}

				if req, ok := params["required"].([]any); ok {
					var required []string
					for _, r := range req {
						if s, ok := r.(string); ok {
							required = append(required, s)
						}
					}
					toolParam.InputSchema.Required = required
				}

				// Preserve schema fields without dedicated Anthropic struct fields (e.g., additionalProperties).
				var extraFields map[string]any
				for key, value := range params {
					if key != "type" && key != "properties" && key != "required" {
						if extraFields == nil {
							extraFields = make(map[string]any)
						}
						extraFields[key] = value
					}
				}
				toolParam.InputSchema.ExtraFields = extraFields
			}

			anthropicTools = append(anthropicTools, anthropic.ToolUnionParam{
				OfTool: &toolParam,
			})

		case string(types.Custom):
			// Anthropic only supports standard function tools, not OpenAI's custom tool extensions.
			return nil, fmt.Errorf("custom tool not supported by Anthropic Claude at index %d", i)

		default:
			return nil, fmt.Errorf("unsupported tool type %s at index %d", discriminator, i)
		}
	}

	return anthropicTools, nil
}

// fromToolChoiceOption converts OpenAI tool_choice to Anthropic ToolChoiceUnionParam.
func fromToolChoiceOption(
	toolChoice *types.ChatCompletionToolChoiceOption,
) (anthropic.ToolChoiceUnionParam, error) {
	if toolChoice == nil {
		// OpenAI defaults to auto when tools are provided but no choice is specified.
		return anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{},
		}, nil
	}

	if stringChoice, err := toolChoice.AsChatCompletionToolChoiceOption0(); err == nil {
		switch stringChoice {
		case types.ChatCompletionToolChoiceOption0None:
			return anthropic.ToolChoiceUnionParam{
				OfNone: &anthropic.ToolChoiceNoneParam{},
			}, nil
		case types.ChatCompletionToolChoiceOption0Auto:
			return anthropic.ToolChoiceUnionParam{
				OfAuto: &anthropic.ToolChoiceAutoParam{},
			}, nil
		case types.ChatCompletionToolChoiceOption0Required:
			return anthropic.ToolChoiceUnionParam{
				OfAny: &anthropic.ToolChoiceAnyParam{},
			}, nil
		default:
			return anthropic.ToolChoiceUnionParam{}, fmt.Errorf("unsupported tool choice string: %s", stringChoice)
		}
	}

	// Union types require discriminator validation: As*() methods only verify structural
	// compatibility via JSON unmarshaling, not semantic correctness via Type field.
	if namedChoice, err := toolChoice.AsChatCompletionNamedToolChoice(); err == nil {
		if namedChoice.Type == types.ChatCompletionNamedToolChoiceTypeFunction {
			return anthropic.ToolChoiceUnionParam{
				OfTool: &anthropic.ToolChoiceToolParam{
					Name: namedChoice.Function.Name,
				},
			}, nil
		}
	}

	if customChoice, err := toolChoice.AsChatCompletionNamedToolChoiceCustom(); err == nil {
		if customChoice.Type == types.ChatCompletionNamedToolChoiceCustomTypeCustom {
			return anthropic.ToolChoiceUnionParam{}, fmt.Errorf("custom tools not supported by Anthropic Claude")
		}
	}

	// OpenAI's allowed_tools restricts the model to multiple specific functions; Anthropic only
	// supports single-tool restriction. Cannot preserve multi-tool constraint semantics.
	if allowedChoice, err := toolChoice.AsChatCompletionAllowedToolsChoice(); err == nil {
		if allowedChoice.Type == types.AllowedTools {
			// AllowedTools transformation: OpenAI's allowed_tools restricts model to specific
			// function subset via array of tool definitions. Anthropic only supports restricting
			// to a single tool via ToolChoiceToolParam with tool name. Cannot map array-based
			// restrictions to single-tool restriction without losing expressiveness.
			return anthropic.ToolChoiceUnionParam{}, fmt.Errorf("allowed_tools choice not supported by Anthropic Claude (only single tool restriction via named choice)")
		}
	}

	return anthropic.ToolChoiceUnionParam{
		OfAuto: &anthropic.ToolChoiceAutoParam{},
	}, nil
}

// toChatCompletionMessageToolCalls converts Anthropic tool use blocks to OpenAI tool calls format (non-streaming).
//
// Returns *[]Item (pointer to slice) to match ChatCompletionResponseMessage.ToolCalls field type,
// which requires a pointer to distinguish nil (field omitted from JSON via omitempty) from an
// empty slice (serialized as "tool_calls": []). Current implementation returns nil when no tool_use
// blocks exist, or &slice when tools are present.
func toChatCompletionMessageToolCalls(content []anthropic.ContentBlockUnion) (*types.ChatCompletionMessageToolCalls, error) {
	var toolCallItems []types.ChatCompletionMessageToolCalls_Item

	for _, block := range content {
		// AsAny() returns the concrete type for Anthropic SDK union discrimination.
		switch variant := block.AsAny().(type) {
		case anthropic.ToolUseBlock:
			// CustomToolCall transformation: OpenAI's ChatCompletionMessageCustomToolCall allows
			// arbitrary custom tool definitions beyond standard function calling. Anthropic
			// only supports standard function-based tools with no custom tool extension mechanism.

			// OpenAI spec requires tool_call_id; generate fallback if missing.
			toolCallID := variant.ID
			if toolCallID == "" {
				toolCallID = newToolCallID()
			}

			// OpenAI expects JSON-encoded string, not json.RawMessage.
			arguments := "{}"
			if len(variant.Input) > 0 {
				arguments = string(variant.Input)
			}

			toolCall := types.ChatCompletionMessageToolCall{
				Id:   toolCallID,
				Type: types.ChatCompletionMessageToolCallTypeFunction,
				Function: struct {
					Arguments string `json:"arguments"`
					Name      string `json:"name"`
				}{
					Name:      variant.Name,
					Arguments: arguments,
				},
			}

			// Generated union types require explicit constructor method.
			var item types.ChatCompletionMessageToolCalls_Item
			if err := item.FromChatCompletionMessageToolCall(toolCall); err != nil {
				return nil, fmt.Errorf("create tool call item: %w", err)
			}
			toolCallItems = append(toolCallItems, item)
		}
	}

	if len(toolCallItems) == 0 {
		return nil, nil
	}

	result := types.ChatCompletionMessageToolCalls(toolCallItems)
	return &result, nil
}

// newToolCallID generates an OpenAI-style tool call ID (format: call_<8-char-uuid>).
func newToolCallID() string {
	return fmt.Sprintf("call_%s", uuid.New().String()[:8])
}
