// Package anthropicclaude adapts OpenAI requests to Anthropic, enabling OpenAI SDK clients
// to work with Claude models without code changes.
//
// The adapter handles:
//
//   - Message transformation: System/developer messages are hoisted to Anthropic's System field
//     while preserving conversation order. Tool messages are merged when consecutive (required
//     by Anthropic's role alternation rules).
//
//   - Tool calling: Bidirectional tool call ID preservation and index translation. Anthropic uses
//     mixed content indices (text=0, tool=1, ...) while OpenAI uses tool-only indices
//     (tool=0, tool=1).
//
//   - Content blocks: Maps between OpenAI's content parts and Anthropic's content blocks. Some
//     Anthropic-specific blocks (ServerToolUseBlock, ...) cannot be mapped to OpenAI responses
//     as they would break conversation round-trips.
//
//   - Streaming: Translates Anthropic's SSE events to OpenAI's chunk with proper state management
//     for tool call indices and metadata accumulation.
//
// # Adapters
//
// CreateChatCompletionAdapter: OpenAI CreateChatCompletion → Anthropic Messages
package anthropicclaude
