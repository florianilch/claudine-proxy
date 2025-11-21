package openaiadapter

//go:generate go run ./types/generate.go

import (
	"context"
	"iter"
	"net/http"

	"github.com/florianilch/claudine-proxy/internal/openaiadapter/types"
)

// Adapter defines the contract for transforming client requests to provider API calls.
//
// Type parameters allow the interface to express transformation contracts for different
// request/response shapes while maintaining compile-time type safety.
//
// Type parameters:
//   - TRequest:  Client-specific request structure
//   - TResponse: Client-specific response structure
//   - TChunk:    Client-specific streaming chunk protocol
type Adapter[TRequest, TResponse, TChunk any] interface {
	// ProcessRequest transforms the client request, calls the provider API, and returns
	// the transformed response. Implementations should remain stateless.
	ProcessRequest(ctx context.Context, clientReq TRequest, transport http.RoundTripper) (*TResponse, error)

	// ProcessStreamingRequest transforms the client request, calls the provider streaming API,
	// and returns an iterator of transformed chunks. Implementations should remain stateless.
	ProcessStreamingRequest(ctx context.Context, clientReq TRequest, transport http.RoundTripper) (iter.Seq2[*TChunk, error], error)
}

// Type aliases for OpenAI-compatible chat completion operations.
// Request/response types are generated from OpenAPI spec (see types package).
// CreateChatCompletionAdapter is the concrete adapter interface for this operation.
type (
	CreateChatCompletionRequest  = types.CreateChatCompletionRequest
	CreateChatCompletionResponse = types.CreateChatCompletionResponse
	CreateChatCompletionChunk    = types.CreateChatCompletionStreamResponse

	CreateChatCompletionAdapter = Adapter[
		CreateChatCompletionRequest,
		CreateChatCompletionResponse,
		CreateChatCompletionChunk,
	]
)

// Type aliases for OpenAI-compatible error responses.
// Error types are generated from OpenAPI spec (see types package).
type (
	Error         = types.Error
	ErrorResponse = types.ErrorResponse
	ErrorEvent    = types.ErrorEvent
)
