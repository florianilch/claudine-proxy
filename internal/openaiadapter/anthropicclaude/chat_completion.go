package anthropicclaude

import (
	"context"
	"fmt"
	"iter"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/florianilch/claudine-proxy/internal/openaiadapter"
	"github.com/florianilch/claudine-proxy/internal/openaiadapter/types"
)

// CreateChatCompletionAdapter transforms generic OpenAI chat completion requests to Anthropic Messages.
//
// Anthropic-specific transformations:
//   - System messages: Moved to dedicated System field (cleaner than Gemini's SystemInstruction)
//   - Developer messages: Merged with system prompts (no developer role equivalent)
//   - Tool call IDs: Preserved bidirectionally for proper request/response matching
//   - Streaming: Anthropic returns delta-based events similar to OpenAI protocol
type CreateChatCompletionAdapter struct{}

// Compile-time interface implementation check.
var _ openaiadapter.CreateChatCompletionAdapter = (*CreateChatCompletionAdapter)(nil)

// ToolIndexMapping maps Anthropic content block indices to OpenAI tool call indices.
// Anthropic uses mixed content indices (text=0, tool=1, thinking=2...) while OpenAI uses
// tool-only indices (tool=0, tool=1...). ID and Name are stored for initial chunk emission.
type ToolIndexMapping struct {
	ID                string
	Name              string
	OpenAIToolCallIdx int
}

// StreamingResponseContext maintains state across streaming chunks for proper index
// translation and metadata accumulation in a single response.
type StreamingResponseContext struct {
	// NextToolCallIndex maintains tool call index continuity across chunks.
	// Tool call indices are scoped per response, not reset per chunk.
	NextToolCallIndex int

	// AnthropicToolIndex maps Anthropic content block indices to OpenAI tool call indices.
	// Stores ID and name for initial chunk emission during tool_use blocks.
	AnthropicToolIndex map[int64]ToolIndexMapping

	// AnthropicMessage accumulates message metadata via selective Accumulate() calls.
	// Only MessageStart/MessageDelta events are accumulated to avoid expensive content arrays.
	AnthropicMessage anthropic.Message
}

// NewCreateChatCompletionAdapter creates a new chat completion adapter.
func NewCreateChatCompletionAdapter() *CreateChatCompletionAdapter {
	return &CreateChatCompletionAdapter{}
}

// ProcessRequest handles non-streaming chat completion by validating the request,
// calling Anthropic's API and transforming the response back to OpenAI format.
func (a *CreateChatCompletionAdapter) ProcessRequest(
	ctx context.Context,
	clientReq openaiadapter.CreateChatCompletionRequest,
	transport http.RoundTripper,
) (*openaiadapter.CreateChatCompletionResponse, error) {
	if err := a.validateRequest(clientReq); err != nil {
		return nil, toChatCompletionError(err)
	}

	providerResp, err := a.callProviderAPI(ctx, clientReq, transport)
	if err != nil {
		return nil, toChatCompletionError(err)
	}

	resp, err := a.transformResponse(providerResp)
	if err != nil {
		return nil, toChatCompletionError(err)
	}
	return resp, nil
}

// ProcessStreamingRequest handles streaming chat completion by validating the request,
// calling Anthropic's streaming API and transforming events to OpenAI chunks via iterator.
func (a *CreateChatCompletionAdapter) ProcessStreamingRequest(
	ctx context.Context,
	clientReq openaiadapter.CreateChatCompletionRequest,
	transport http.RoundTripper,
) (iter.Seq2[*openaiadapter.CreateChatCompletionChunk, error], error) {
	if err := a.validateRequest(clientReq); err != nil {
		return nil, toChatCompletionError(err)
	}

	stream, err := a.callProviderAPIStreaming(ctx, clientReq, transport)
	if err != nil {
		return nil, toChatCompletionError(err)
	}

	return func(yield func(*openaiadapter.CreateChatCompletionChunk, error) bool) {
		defer func() { _ = stream.Close() }()

		streamingContext := StreamingResponseContext{
			NextToolCallIndex:  0,
			AnthropicToolIndex: make(map[int64]ToolIndexMapping),
		}

		for stream.Next() {
			event := stream.Current()

			chunk, err := a.transformStreamEvent(&streamingContext, event)
			if err != nil {
				yield(nil, toChatCompletionError(err))
				return
			}

			if chunk == nil {
				continue
			}

			if !yield(chunk, nil) {
				return
			}
		}

		if err := stream.Err(); err != nil {
			yield(nil, toChatCompletionError(err))
			return
		}
	}, nil
}

// validateRequest performs minimal validation of universally required fields.
func (a *CreateChatCompletionAdapter) validateRequest(
	clientReq openaiadapter.CreateChatCompletionRequest,
) error {
	if clientReq.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(clientReq.Messages) == 0 {
		return fmt.Errorf("messages array cannot be empty")
	}

	return nil
}

// callProviderAPI transforms the request and calls Anthropic's non-streaming API.
func (a *CreateChatCompletionAdapter) callProviderAPI(
	ctx context.Context,
	clientReq openaiadapter.CreateChatCompletionRequest,
	transport http.RoundTripper,
) (*anthropic.Message, error) {
	client, err := newClient(transport)
	if err != nil {
		return nil, fmt.Errorf("initialize Anthropic client for non-streaming request: %w", err)
	}

	// Transform and separate OpenAI messages - preserves order while hoisting system prompts
	transformed, err := fromChatCompletionRequestMessages(clientReq.Messages)
	if err != nil {
		return nil, fmt.Errorf("transform messages: %w", err)
	}
	systemPrompts, messages := hoistSystemPrompts(transformed)

	params, err := buildGenerationParams(clientReq)
	if err != nil {
		return nil, fmt.Errorf("build generation params: %w", err)
	}
	params.Messages = messages
	params.System = systemPrompts

	message, err := client.Messages.New(ctx, params)
	if err != nil {
		return nil, err
	}

	return message, nil
}

// callProviderAPIStreaming transforms the request and calls Anthropic's streaming API.
func (a *CreateChatCompletionAdapter) callProviderAPIStreaming(
	ctx context.Context,
	clientReq openaiadapter.CreateChatCompletionRequest,
	transport http.RoundTripper,
) (*ssestream.Stream[anthropic.MessageStreamEventUnion], error) {
	client, err := newClient(transport)
	if err != nil {
		return nil, fmt.Errorf("initialize Anthropic client for streaming request: %w", err)
	}

	// Transform and separate OpenAI messages - preserves order while hoisting system prompts
	transformed, err := fromChatCompletionRequestMessages(clientReq.Messages)
	if err != nil {
		return nil, fmt.Errorf("transform messages: %w", err)
	}
	systemPrompts, messages := hoistSystemPrompts(transformed)

	params, err := buildGenerationParams(clientReq)
	if err != nil {
		return nil, fmt.Errorf("build generation params: %w", err)
	}
	params.Messages = messages
	params.System = systemPrompts

	stream := client.Messages.NewStreaming(ctx, params)
	return stream, nil
}

// transformResponse converts Anthropic message to OpenAI chat completion format.
func (a *CreateChatCompletionAdapter) transformResponse(
	providerResp *anthropic.Message,
) (*openaiadapter.CreateChatCompletionResponse, error) {
	var messageContent *string
	var toolCalls *types.ChatCompletionMessageToolCalls

	// Extract text content (including refusals)
	//
	// ThinkingBlock transformation: Anthropic's extended thinking/reasoning content in ThinkingBlock
	// cannot be mapped to OpenAI responses without breaking conversation round-trips. If mapped to
	// regular content, clients would send thinking back as normal assistant messages in history,
	// confusing the model.
	//
	// Citations transformation: Anthropic's Citations within TextBlock provide source attribution.
	// OpenAI has no equivalent citation metadata in chat completion response format.
	//
	// RedactedThinkingBlock transformation: Anthropic's redacted thinking blocks for privacy.
	// OpenAI has no equivalent redacted content mechanism in chat completion response format.
	textContent := textFromAnthropicContentBlocks(providerResp.Content)
	if textContent != "" {
		messageContent = &textContent
	}

	// Extract tool calls
	//
	// ServerToolUseBlock transformation: Anthropic's server-side tool execution blocks.
	// OpenAI chat completion has no equivalent server-side tool mechanism.
	//
	// WebSearchToolResultBlock transformation: Anthropic's web search tool results.
	// OpenAI chat completion has no equivalent built-in web search tool result type.
	var err error
	toolCalls, err = toChatCompletionMessageToolCalls(providerResp.Content)
	if err != nil {
		return nil, fmt.Errorf("extract tool calls: %w", err)
	}

	message := types.ChatCompletionResponseMessage{
		Role:      types.ChatCompletionResponseMessageRoleAssistant,
		Content:   messageContent,
		Refusal:   nil, // Refusals are returned as content with finish_reason="content_filter"
		ToolCalls: toolCalls,
	}

	choice := types.CreateChatCompletionResponseChoice{
		FinishReason: toFinishReason(providerResp.StopReason),
		Index:        0,
		Logprobs:     nil, // Anthropic doesn't provide logprobs
		Message:      message,
	}

	// Generate fallback ID if Anthropic doesn't provide one
	responseID := providerResp.ID
	if responseID == "" {
		responseID = newResponseID()
	}

	response := openaiadapter.CreateChatCompletionResponse{
		Choices: []types.CreateChatCompletionResponseChoice{choice},
		Created: 0, // Anthropic SDK doesn't provide created timestamp
		Id:      responseID,
		Model:   string(providerResp.Model),
		Object:  types.ChatCompletion,
		Usage:   toCompletionUsage(providerResp.Usage),
	}

	return &response, nil
}

// newStreamChunk creates an OpenAI streaming chunk with consistent defaults.
func (a *CreateChatCompletionAdapter) newStreamChunk(
	delta types.ChatCompletionStreamResponseDelta,
	finishReason *types.CreateChatCompletionStreamResponseChoiceFinishReason,
	responseID string,
	model string,
	usage *types.CompletionUsage,
) *openaiadapter.CreateChatCompletionChunk {
	return &openaiadapter.CreateChatCompletionChunk{
		Choices: []types.CreateChatCompletionStreamResponseChoice{{
			Delta:        delta,
			FinishReason: finishReason,
			Index:        0,
			Logprobs:     nil,
		}},
		Created: 0,
		Id:      responseID,
		Model:   model,
		Object:  types.ChatCompletionChunk,
		Usage:   usage,
	}
}

// transformStreamEvent converts an Anthropic stream event to OpenAI chunk format.
// Selectively accumulates message metadata from MessageStart/MessageDelta events while
// skipping ContentBlock events to avoid expensive content array building. Handles index
// translation between Anthropic's mixed content indices and OpenAI's tool-only indices.
func (a *CreateChatCompletionAdapter) transformStreamEvent(
	streamingContext *StreamingResponseContext,
	event anthropic.MessageStreamEventUnion,
) (*openaiadapter.CreateChatCompletionChunk, error) {

	// Event lifecycle transformation:
	//   message_start       → emit role
	//   content_block_start → emit tool metadata (tool_use only), skip text/thinking
	//   content_block_delta → emit text/tool JSON deltas, skip thinking/citations/signatures
	//   content_block_stop  → skip (no-op)
	//   message_delta       → emit finish_reason + usage (final data arrives here)
	//   message_stop        → skip (termination signal, no data)
	switch eventType := event.AsAny().(type) {
	// First event: provides message metadata (ID, Model, initial Usage)
	case anthropic.MessageStartEvent:
		// Accumulate message metadata (ID, Model, Usage) - skips content arrays
		if err := streamingContext.AnthropicMessage.Accumulate(event); err != nil {
			return nil, fmt.Errorf("accumulate message start: %w", err)
		}

		// Generate response ID if not provided by Anthropic
		if streamingContext.AnthropicMessage.ID == "" {
			streamingContext.AnthropicMessage.ID = newResponseID()
		}

		// OpenAI protocol: first chunk contains only role, subsequent chunks omit role
		assistantRole := types.ChatCompletionStreamResponseDeltaRoleAssistant
		return a.newStreamChunk(
			types.ChatCompletionStreamResponseDelta{Role: &assistantRole},
			nil, // Finish reason comes in MessageDeltaEvent
			streamingContext.AnthropicMessage.ID,
			string(streamingContext.AnthropicMessage.Model),
			nil, // Usage comes in MessageDeltaEvent
		), nil

	// New content block begins (text/tool_use/thinking)
	case anthropic.ContentBlockStartEvent:
		if eventType.ContentBlock.Type == "text" {
			return nil, nil // Content comes in delta events
		}

		if eventType.ContentBlock.Type == "tool_use" {
			// OpenAI requires initial chunk with id/name/args="" before JSON deltas
			toolID := eventType.ContentBlock.ID
			toolName := eventType.ContentBlock.Name

			// Store mapping for later InputJSONDelta events to resolve correct OpenAI tool index
			openaiIdx := streamingContext.NextToolCallIndex
			streamingContext.AnthropicToolIndex[eventType.Index] = ToolIndexMapping{
				ID:                toolID,
				Name:              toolName,
				OpenAIToolCallIdx: openaiIdx,
			}
			streamingContext.NextToolCallIndex++

			emptyArgs := ""
			toolCall := types.ChatCompletionMessageToolCallChunk{
				Id:    &toolID,
				Index: openaiIdx,
				Type:  types.ChatCompletionMessageToolCallChunkTypeFunction,
				Function: &struct {
					Arguments *string `json:"arguments,omitempty"`
					Name      *string `json:"name,omitempty"`
				}{
					Name:      &toolName,
					Arguments: &emptyArgs,
				},
			}

			var toolCallItem types.ChatCompletionStreamResponseDelta_ToolCalls_Item
			if err := toolCallItem.FromChatCompletionMessageToolCallChunk(toolCall); err != nil {
				return nil, fmt.Errorf("create initial tool call chunk: %w", err)
			}

			toolCalls := []types.ChatCompletionStreamResponseDelta_ToolCalls_Item{toolCallItem}
			delta := types.ChatCompletionStreamResponseDelta{ToolCalls: &toolCalls}

			return a.newStreamChunk(
				delta,
				nil, // Finish reason comes in MessageDeltaEvent
				streamingContext.AnthropicMessage.ID,
				string(streamingContext.AnthropicMessage.Model),
				nil, // Usage comes in MessageDeltaEvent
			), nil
		}

		return nil, nil // Non-mappable blocks (thinking, etc.)

	// Incremental content: text fragments or tool JSON deltas
	case anthropic.ContentBlockDeltaEvent:
		var delta types.ChatCompletionStreamResponseDelta

		switch deltaVariant := eventType.Delta.AsAny().(type) {
		case anthropic.TextDelta:
			if deltaVariant.Text != "" {
				delta.Content = &deltaVariant.Text
			}
		case anthropic.InputJSONDelta:
			// Retrieve OpenAI tool index from mapping created in ContentBlockStartEvent
			toolMetadata, exists := streamingContext.AnthropicToolIndex[eventType.Index]
			if !exists {
				return nil, fmt.Errorf("received InputJSONDelta for unknown tool at index %d", eventType.Index)
			}

			toolCall := types.ChatCompletionMessageToolCallChunk{
				Index: toolMetadata.OpenAIToolCallIdx,
				Function: &struct {
					Arguments *string `json:"arguments,omitempty"`
					Name      *string `json:"name,omitempty"`
				}{
					Arguments: &deltaVariant.PartialJSON,
				},
			}

			var toolCallItem types.ChatCompletionStreamResponseDelta_ToolCalls_Item
			if err := toolCallItem.FromChatCompletionMessageToolCallChunk(toolCall); err != nil {
				return nil, fmt.Errorf("create tool argument delta chunk: %w", err)
			}

			toolCalls := []types.ChatCompletionStreamResponseDelta_ToolCalls_Item{toolCallItem}
			delta.ToolCalls = &toolCalls
		case anthropic.ThinkingDelta:
			// Skip: would break round-trips (clients would echo thinking as regular messages)
			return nil, nil
		case anthropic.CitationsDelta:
			// Skip: no OpenAI equivalent
			return nil, nil
		case anthropic.SignatureDelta:
			// Skip: no OpenAI equivalent
			return nil, nil
		}

		if delta.Content == nil && delta.ToolCalls == nil {
			return nil, nil
		}

		return a.newStreamChunk(
			delta,
			nil, // No finish reason yet
			streamingContext.AnthropicMessage.ID,
			string(streamingContext.AnthropicMessage.Model),
			nil, // No usage yet
		), nil

	// Content block finished
	case anthropic.ContentBlockStopEvent:
		return nil, nil // Content already streamed via start/delta events

	// StopReason and final OutputTokens arrive here (not in MessageStopEvent)
	case anthropic.MessageDeltaEvent:
		// Accumulate message delta (StopReason, Usage) - skips content arrays
		if err := streamingContext.AnthropicMessage.Accumulate(event); err != nil {
			return nil, fmt.Errorf("accumulate message delta: %w", err)
		}

		// Final chunk with finish_reason and usage (content already streamed in deltas)
		finishReason := toFinishReasonStreaming(streamingContext.AnthropicMessage.StopReason)
		return a.newStreamChunk(
			types.ChatCompletionStreamResponseDelta{},
			&finishReason,
			streamingContext.AnthropicMessage.ID,
			string(streamingContext.AnthropicMessage.Model),
			toCompletionUsage(streamingContext.AnthropicMessage.Usage),
		), nil

	// Termination signal (contains no data we need)
	case anthropic.MessageStopEvent:
		return nil, nil

	// Unknown or future event type
	default:
		return nil, nil
	}
}

// hoistSystemPrompts separates system/developer messages from conversation messages.
// Anthropic requires system prompts in a dedicated System field rather than the Messages array.
func hoistSystemPrompts(transformed []transformedMessage) ([]anthropic.TextBlockParam, []anthropic.MessageParam) {
	var systemPrompts []anthropic.TextBlockParam
	var messages []anthropic.MessageParam

	for _, msg := range transformed {
		switch msg.Role {
		case string(types.System), string(types.ChatCompletionRequestDeveloperMessageRoleDeveloper):
			if textBlock, ok := msg.Content.(*anthropic.TextBlockParam); ok {
				systemPrompts = append(systemPrompts, *textBlock)
			}
		case string(types.User), string(types.ChatCompletionRequestAssistantMessageRoleAssistant), string(types.Tool):
			if msgParam, ok := msg.Content.(*anthropic.MessageParam); ok {
				messages = append(messages, *msgParam)
			}
		}
	}

	return systemPrompts, messages
}
