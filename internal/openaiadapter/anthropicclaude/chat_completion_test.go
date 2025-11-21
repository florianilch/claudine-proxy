package anthropicclaude_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/florianilch/claudine-proxy/internal/openaiadapter/anthropicclaude"
	"github.com/florianilch/claudine-proxy/internal/openaiadapter/types"
)

// mockTransport captures HTTP requests and returns canned responses
type mockTransport struct {
	capturedRequest *http.Request
	capturedBody    []byte
	responseBody    string
	responseStatus  int
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	m.capturedRequest = req
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	m.capturedBody = body
	if err := req.Body.Close(); err != nil {
		return nil, err
	}

	// SSE requests need text/event-stream content type
	contentType := "application/json"
	if req.Header.Get("Accept") == "text/event-stream" {
		contentType = "text/event-stream"
	}

	return &http.Response{
		StatusCode: m.responseStatus,
		Body:       io.NopCloser(strings.NewReader(m.responseBody)),
		Header:     http.Header{"Content-Type": []string{contentType}},
		Request:    req,
	}, nil
}

// turn represents a single request-response cycle for non-streaming tests.
// Each field captures a stage in the adapter pipeline for assertion.
type turn struct {
	OpenAIRequest           json.RawMessage `json:"openaiRequest"`           // What client sends to adapter
	AnthropicRequest        json.RawMessage `json:"anthropicRequest"`        // What adapter sends to Anthropic
	AnthropicResponse       json.RawMessage `json:"anthropicResponse"`       // What Anthropic returns
	AnthropicResponseStatus int             `json:"anthropicResponseStatus"` // HTTP status code for response (default: 200)
	OpenAIResponse          json.RawMessage `json:"openaiResponse"`          // What adapter returns to client
}

// streamingTurn represents a single streaming request-response cycle.
// Each field captures a stage in the streaming adapter pipeline for assertion.
type streamingTurn struct {
	OpenAIRequest    json.RawMessage   `json:"openaiRequest"`    // What client sends to adapter
	AnthropicRequest json.RawMessage   `json:"anthropicRequest"` // What adapter sends to Anthropic
	AnthropicSSE     []string          `json:"anthropicSSE"`     // SSE event stream lines from Anthropic
	OpenAIChunks     []json.RawMessage `json:"openaiChunks"`     // OpenAI chunks adapter yields
}

// fixture represents a test case loaded from a JSON file.
type fixture[T any] struct {
	Name  string
	Turns []T
}

// normalizeJSON unmarshals and remarshals JSON to normalize whitespace
func normalizeJSON(t *testing.T, s string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("Invalid JSON: %v\nJSON: %s", err, s)
	}
	normalized, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}
	return string(normalized)
}

// assertJSONEqual compares two JSON strings for semantic equality.
func assertJSONEqual(t *testing.T, got, want string) {
	t.Helper()
	gotNorm := normalizeJSON(t, got)
	wantNorm := normalizeJSON(t, want)
	if gotNorm != wantNorm {
		t.Errorf("JSON mismatch:\ngot:  %s\nwant: %s", gotNorm, wantNorm)
	}
}

// loadFixtures loads and parses all test fixture files matching the given pattern.
func loadFixtures[T any](t *testing.T, pattern string) []fixture[T] {
	t.Helper()

	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("Failed to glob pattern %s: %v", pattern, err)
	}

	if len(matches) == 0 {
		t.Fatalf("No fixture files found for pattern: %s", pattern)
	}

	sort.Strings(matches)

	var fixtures []fixture[T]
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read fixture %s: %v", path, err)
		}

		// Normalize JSON: unmarshal and re-marshal compactly
		// This ensures json.RawMessage fields contain compact JSON matching real API responses
		var intermediate any
		if err := json.Unmarshal(data, &intermediate); err != nil {
			t.Fatalf("Failed to unmarshal fixture %s for normalization: %v", path, err)
		}
		compactData, err := json.Marshal(intermediate)
		if err != nil {
			t.Fatalf("Failed to re-marshal fixture %s: %v", path, err)
		}

		var turns []T
		if err := json.Unmarshal(compactData, &turns); err != nil {
			t.Fatalf("Failed to unmarshal fixture %s: %v", path, err)
		}

		basename := filepath.Base(path)
		name := strings.TrimSuffix(basename, filepath.Ext(basename))

		fixtures = append(fixtures, fixture[T]{
			Name:  name,
			Turns: turns,
		})
	}

	return fixtures
}

func TestCreateChatCompletionAdapter_Buffered(t *testing.T) {
	t.Parallel()
	fixtures := loadFixtures[turn](t, "testdata/buffered/*.json")

	for _, fix := range fixtures {
		t.Run(fix.Name, func(t *testing.T) {
			t.Parallel()
			adapter := anthropicclaude.NewCreateChatCompletionAdapter()

			ctx := context.Background()

			for i, turn := range fix.Turns {
				t.Logf("Turn %d", i+1)

				status := turn.AnthropicResponseStatus
				if status == 0 {
					status = http.StatusOK
				}

				mock := &mockTransport{
					responseBody:   string(turn.AnthropicResponse),
					responseStatus: status,
				}

				var openaiReq types.CreateChatCompletionRequest
				if err := json.Unmarshal(turn.OpenAIRequest, &openaiReq); err != nil {
					t.Fatalf("Failed to parse openaiRequest: %v", err)
				}

				response, err := adapter.ProcessRequest(ctx, openaiReq, mock)

				if string(turn.AnthropicRequest) != "null" {
					if !strings.Contains(mock.capturedRequest.URL.Path, "/messages") {
						t.Errorf("Expected request to /messages endpoint, got: %s", mock.capturedRequest.URL.Path)
					}

					assertJSONEqual(t, string(mock.capturedBody), string(turn.AnthropicRequest))
				}

				// Handle both success and error responses
				if err != nil {
					var errorResponse *types.ErrorResponse
					if !errors.As(err, &errorResponse) {
						t.Fatalf("Expected types.ErrorResponse, got: %T", err)
					}
					gotResponse, marshalErr := json.Marshal(errorResponse)
					if marshalErr != nil {
						t.Fatalf("Failed to marshal error response: %v", marshalErr)
					}
					assertJSONEqual(t, string(gotResponse), string(turn.OpenAIResponse))
				} else {
					gotResponse, marshalErr := json.Marshal(response)
					if marshalErr != nil {
						t.Fatalf("Failed to marshal response: %v", marshalErr)
					}
					assertJSONEqual(t, string(gotResponse), string(turn.OpenAIResponse))
				}
			}
		})
	}
}

func TestCreateChatCompletionAdapter_Streaming(t *testing.T) {
	t.Parallel()
	fixtures := loadFixtures[streamingTurn](t, "testdata/streaming/*.json")

	for _, fix := range fixtures {
		t.Run(fix.Name, func(t *testing.T) {
			t.Parallel()
			adapter := anthropicclaude.NewCreateChatCompletionAdapter()

			ctx := context.Background()

			for i, turn := range fix.Turns {
				t.Logf("Turn %d", i+1)

				// Setup mock transport with SSE response (join array into string)
				mock := &mockTransport{
					responseBody:   strings.Join(turn.AnthropicSSE, "\n"),
					responseStatus: http.StatusOK,
				}

				var openaiReq types.CreateChatCompletionRequest
				if err := json.Unmarshal(turn.OpenAIRequest, &openaiReq); err != nil {
					t.Fatalf("Failed to parse openaiRequest: %v", err)
				}

				stream, err := adapter.ProcessStreamingRequest(ctx, openaiReq, mock)

				// Handle both success and error responses; handle streaming errors during streaming
				if err != nil {
					var errorResponse *types.ErrorResponse
					if !errors.As(err, &errorResponse) {
						t.Fatalf("Expected types.ErrorResponse, got: %T", err)
					}
					gotResponse, marshalErr := json.Marshal(errorResponse)
					if marshalErr != nil {
						t.Fatalf("Failed to marshal error response: %v", marshalErr)
					}
					assertJSONEqual(t, string(gotResponse), string(turn.OpenAIChunks[0]))
				} else {
					if string(turn.AnthropicRequest) != "null" {
						if !strings.Contains(mock.capturedRequest.URL.Path, "/messages") {
							t.Errorf("Expected request to /messages endpoint, got: %s", mock.capturedRequest.URL.Path)
						}

						assertJSONEqual(t, string(mock.capturedBody), string(turn.AnthropicRequest))
					}

					var chunks []string
					for chunk, err := range stream {
						if err != nil {
							var errorResponse *types.ErrorResponse
							if !errors.As(err, &errorResponse) {
								t.Fatalf("Expected types.ErrorResponse, got: %T", err)
							}
							chunkJSON, marshalErr := json.Marshal(errorResponse)
							if marshalErr != nil {
								t.Fatalf("Failed to marshal error chunk: %v", marshalErr)
							}
							chunks = append(chunks, string(chunkJSON))
							break // Errors terminate stream (no more chunks expected)
						}
						chunkJSON, err := json.Marshal(chunk)
						if err != nil {
							t.Fatalf("Failed to marshal chunk: %v", err)
						}
						chunks = append(chunks, string(chunkJSON))
					}

					if len(chunks) != len(turn.OpenAIChunks) {
						t.Errorf("Chunk count mismatch: got %d, want %d", len(chunks), len(turn.OpenAIChunks))
						t.Fatalf("Got chunks:\n%s", strings.Join(chunks, "\n"))
					}

					for j, wantChunk := range turn.OpenAIChunks {
						assertJSONEqual(t, chunks[j], string(wantChunk))
					}
				}
			}
		})
	}
}

func BenchmarkCreateChatCompletion_Buffered(b *testing.B) {
	data, err := os.ReadFile("testdata/buffered/tool_use.json")
	if err != nil {
		b.Fatalf("Failed to read fixture: %v", err)
	}

	var turns []turn
	if err := json.Unmarshal(data, &turns); err != nil {
		b.Fatalf("Failed to unmarshal fixture: %v", err)
	}

	if len(turns) == 0 {
		b.Fatal("No turns in fixture")
	}

	firstTurn := turns[0]
	var openaiReq types.CreateChatCompletionRequest
	if err := json.Unmarshal(firstTurn.OpenAIRequest, &openaiReq); err != nil {
		b.Fatalf("Failed to parse openaiRequest: %v", err)
	}

	adapter := anthropicclaude.NewCreateChatCompletionAdapter()
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		mock := &mockTransport{
			responseBody:   string(firstTurn.AnthropicResponse),
			responseStatus: http.StatusOK,
		}

		_, err := adapter.ProcessRequest(ctx, openaiReq, mock)
		if err != nil {
			b.Fatalf("ProcessRequest failed: %v", err)
		}
	}
}

func BenchmarkCreateChatCompletion_Streaming(b *testing.B) {
	data, err := os.ReadFile("testdata/streaming/tool_use_stream.json")
	if err != nil {
		b.Fatalf("Failed to read fixture: %v", err)
	}

	var turns []streamingTurn
	if err := json.Unmarshal(data, &turns); err != nil {
		b.Fatalf("Failed to unmarshal fixture: %v", err)
	}

	if len(turns) == 0 {
		b.Fatal("No turns in fixture")
	}

	firstTurn := turns[0]
	var openaiReq types.CreateChatCompletionRequest
	if err := json.Unmarshal(firstTurn.OpenAIRequest, &openaiReq); err != nil {
		b.Fatalf("Failed to parse openaiRequest: %v", err)
	}

	adapter := anthropicclaude.NewCreateChatCompletionAdapter()
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		mock := &mockTransport{
			responseBody:   strings.Join(firstTurn.AnthropicSSE, "\n"),
			responseStatus: http.StatusOK,
		}

		stream, err := adapter.ProcessStreamingRequest(ctx, openaiReq, mock)
		if err != nil {
			b.Fatalf("ProcessStreamingRequest failed: %v", err)
		}

		// Streams must be fully consumed to measure realistic performance
		for chunk, err := range stream {
			if err != nil {
				b.Fatalf("Stream error: %v", err)
			}
			_ = chunk
		}
	}
}
