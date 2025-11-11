//go:build goexperiment.jsonv2

package proxy

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// mockAnthropicTransport returns pre-recorded responses without network calls.
type mockAnthropicTransport struct {
	responseBody   string
	responseStatus int
	isStreaming    bool
}

func (m *mockAnthropicTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	contentType := "application/json"
	if m.isStreaming {
		contentType = "text/event-stream"
	}

	return &http.Response{
		StatusCode: m.responseStatus,
		Body:       io.NopCloser(strings.NewReader(m.responseBody)),
		Header:     http.Header{"Content-Type": []string{contentType}},
		Request:    req,
	}, nil
}

// mockReadinessChecker always reports ready status for benchmarks.
type mockReadinessChecker struct{}

func (mockReadinessChecker) IsReady() bool {
	return true
}

// streamingTurn represents a single streaming request-response cycle from test fixtures.
type streamingTurn struct {
	OpenAIRequest    json.RawMessage   `json:"openaiRequest"`
	AnthropicRequest json.RawMessage   `json:"anthropicRequest"`
	AnthropicSSE     []string          `json:"anthropicSSE"`
	OpenAIChunks     []json.RawMessage `json:"openaiChunks"`
}

// bufferedTurn represents a single non-streaming request-response cycle from test fixtures.
type bufferedTurn struct {
	OpenAIRequest           json.RawMessage `json:"openaiRequest"`
	AnthropicRequest        json.RawMessage `json:"anthropicRequest"`
	AnthropicResponse       json.RawMessage `json:"anthropicResponse"`
	AnthropicResponseStatus int             `json:"anthropicResponseStatus"`
	OpenAIResponse          json.RawMessage `json:"openaiResponse"`
}

// loadStreamingFixture loads an existing streaming test fixture and extracts
// the OpenAI request body and Anthropic SSE response for benchmarking.
func loadStreamingFixture(b *testing.B, name string) (openaiReq string, anthropicSSE string) {
	b.Helper()

	path := filepath.Join("..", "openaiadapter", "anthropicclaude", "testdata", "streaming", name)
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("Failed to read fixture %s: %v", name, err)
	}

	var turns []streamingTurn
	if err := json.Unmarshal(data, &turns); err != nil {
		b.Fatalf("Failed to parse fixture %s: %v", name, err)
	}

	if len(turns) == 0 {
		b.Fatalf("No turns in fixture %s", name)
	}

	turn := turns[0]
	openaiReq = string(turn.OpenAIRequest)
	anthropicSSE = strings.Join(turn.AnthropicSSE, "\n")
	return openaiReq, anthropicSSE
}

// loadBufferedFixture loads an existing buffered test fixture and extracts
// the OpenAI request body and Anthropic JSON response for benchmarking.
func loadBufferedFixture(b *testing.B, name string) (openaiReq string, anthropicJSON string) {
	b.Helper()

	path := filepath.Join("..", "openaiadapter", "anthropicclaude", "testdata", "buffered", name)
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("Failed to read fixture %s: %v", name, err)
	}

	var turns []bufferedTurn
	if err := json.Unmarshal(data, &turns); err != nil {
		b.Fatalf("Failed to parse fixture %s: %v", name, err)
	}

	if len(turns) == 0 {
		b.Fatalf("No turns in fixture %s", name)
	}

	turn := turns[0]
	openaiReq = string(turn.OpenAIRequest)
	anthropicJSON = string(turn.AnthropicResponse)
	return openaiReq, anthropicJSON
}

// setupProxyWithMockTransport creates a Proxy with full middleware stack but mocked upstream.
// Suppresses logging to isolate benchmark measurements from I/O overhead.
func setupProxyWithMockTransport(b *testing.B, transport http.RoundTripper) *Proxy {
	b.Helper()

	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	mockTokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"})
	mockHealth := mockReadinessChecker{}

	proxy, err := New(mockTokenSource, mockHealth, WithTransport(transport))
	if err != nil {
		b.Fatalf("Failed to create proxy: %v", err)
	}

	return proxy
}

// consumeSSEStream drains the response body to measure proxy throughput.
// Uses raw byte copy instead of SSE parsing to isolate proxy performance from client overhead.
func consumeSSEStream(b *testing.B, body io.Reader) {
	b.Helper()

	_, err := io.Copy(io.Discard, body)
	if err != nil {
		b.Fatalf("Stream read error: %v", err)
	}
}

// BenchmarkProxyStreaming measures end-to-end streaming latency through
// the OpenAI compatibility layer with multiple scenarios.
// Includes routing, middleware, handler, adapter, and SSE encoding.
// Excludes network latency (mocked transport) and OAuth refresh overhead.
func BenchmarkProxyStreaming(b *testing.B) {
	scenarios := []struct {
		name        string
		fixtureName string
	}{
		{
			name:        "multi_turn",
			fixtureName: "multi_turn_stream.json",
		},
		{
			name:        "tool_use",
			fixtureName: "tool_use_stream.json",
		},
		{
			name:        "mixed_content",
			fixtureName: "mixed_content_stream.json",
		},
	}

	for _, s := range scenarios {
		openaiReq, anthropicSSE := loadStreamingFixture(b, s.fixtureName)

		b.Run(s.name, func(b *testing.B) {
			mockTransport := &mockAnthropicTransport{
				responseBody:   anthropicSSE,
				responseStatus: http.StatusOK,
				isStreaming:    true,
			}

			proxy := setupProxyWithMockTransport(b, mockTransport)
			server := httptest.NewServer(proxy)
			defer server.Close()

			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				resp, err := http.Post(
					server.URL+"/v1/chat/completions",
					"application/json",
					strings.NewReader(openaiReq),
				)
				if err != nil {
					b.Fatalf("Request failed: %v", err)
				}

				if resp.StatusCode != http.StatusOK {
					b.Fatalf("Unexpected status code: %d", resp.StatusCode)
				}

				consumeSSEStream(b, resp.Body)
				_ = resp.Body.Close()
			}
		})
	}
}

// BenchmarkProxyNonStreaming measures end-to-end buffered response latency.
// Provides baseline comparison against streaming benchmarks to isolate SSE overhead.
func BenchmarkProxyNonStreaming(b *testing.B) {
	scenarios := []struct {
		name        string
		fixtureName string
	}{
		{
			name:        "multi_turn",
			fixtureName: "multi_turn.json",
		},
		{
			name:        "tool_use",
			fixtureName: "tool_use.json",
		},
		{
			name:        "mixed_content",
			fixtureName: "mixed_content.json",
		},
	}

	for _, s := range scenarios {
		openaiReq, anthropicJSON := loadBufferedFixture(b, s.fixtureName)

		b.Run(s.name, func(b *testing.B) {
			mockTransport := &mockAnthropicTransport{
				responseBody:   anthropicJSON,
				responseStatus: http.StatusOK,
				isStreaming:    false,
			}

			proxy := setupProxyWithMockTransport(b, mockTransport)
			server := httptest.NewServer(proxy)
			defer server.Close()

			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				resp, err := http.Post(
					server.URL+"/v1/chat/completions",
					"application/json",
					strings.NewReader(openaiReq),
				)
				if err != nil {
					b.Fatalf("Request failed: %v", err)
				}

				if resp.StatusCode != http.StatusOK {
					b.Fatalf("Unexpected status code: %d", resp.StatusCode)
				}

				_, err = io.Copy(io.Discard, resp.Body)
				if err != nil {
					b.Fatalf("Failed to read response: %v", err)
				}
				_ = resp.Body.Close()
			}
		})
	}
}

// BenchmarkProxyStreaming_TTFB measures Time-To-First-Byte for streaming responses.
// TTFB is the most critical latency metric for streaming UX - lower values mean
// better perceived responsiveness as the first chunk arrives faster.
func BenchmarkProxyStreaming_TTFB(b *testing.B) {
	openaiReq, anthropicSSE := loadStreamingFixture(b, "system_stream.json")

	mockTransport := &mockAnthropicTransport{
		responseBody:   anthropicSSE,
		responseStatus: http.StatusOK,
		isStreaming:    true,
	}

	proxy := setupProxyWithMockTransport(b, mockTransport)
	server := httptest.NewServer(proxy)
	defer server.Close()

	b.ReportAllocs()
	b.ResetTimer()

	var totalTTFB time.Duration
	var iterations int
	buf := make([]byte, 1)

	for b.Loop() {
		start := time.Now()

		resp, err := http.Post(
			server.URL+"/v1/chat/completions",
			"application/json",
			strings.NewReader(openaiReq),
		)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}

		// Read first byte to measure TTFB
		_, err = resp.Body.Read(buf)
		if err != nil {
			b.Fatalf("Failed to read first byte: %v", err)
		}

		ttfb := time.Since(start)
		totalTTFB += ttfb
		iterations++

		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	avgTTFB := totalTTFB / time.Duration(iterations)
	b.ReportMetric(float64(avgTTFB.Microseconds()), "µs/ttfb")
}

// BenchmarkProxyConcurrentThroughput_Streaming measures concurrent streaming throughput
// using b.RunParallel to simulate realistic concurrent load. Reports ops/sec and memory
// allocations per request under concurrent execution.
func BenchmarkProxyConcurrentThroughput_Streaming(b *testing.B) {
	openaiReq, anthropicSSE := loadStreamingFixture(b, "system_stream.json")

	mockTransport := &mockAnthropicTransport{
		responseBody:   anthropicSSE,
		responseStatus: http.StatusOK,
		isStreaming:    true,
	}

	proxy := setupProxyWithMockTransport(b, mockTransport)
	server := httptest.NewServer(proxy)
	defer server.Close()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := http.Post(
				server.URL+"/v1/chat/completions",
				"application/json",
				strings.NewReader(openaiReq),
			)
			if err != nil {
				b.Fatalf("Request failed: %v", err)
			}

			if resp.StatusCode != http.StatusOK {
				b.Fatalf("Unexpected status code: %d", resp.StatusCode)
			}

			consumeSSEStream(b, resp.Body)
			_ = resp.Body.Close()
		}
	})
}

// BenchmarkProxyConcurrentThroughput_NonStreaming measures concurrent buffered throughput.
// Provides baseline comparison to isolate streaming overhead under concurrent load.
func BenchmarkProxyConcurrentThroughput_NonStreaming(b *testing.B) {
	openaiReq, anthropicJSON := loadBufferedFixture(b, "system.json")

	mockTransport := &mockAnthropicTransport{
		responseBody:   anthropicJSON,
		responseStatus: http.StatusOK,
		isStreaming:    false,
	}

	proxy := setupProxyWithMockTransport(b, mockTransport)
	server := httptest.NewServer(proxy)
	defer server.Close()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := http.Post(
				server.URL+"/v1/chat/completions",
				"application/json",
				strings.NewReader(openaiReq),
			)
			if err != nil {
				b.Fatalf("Request failed: %v", err)
			}

			if resp.StatusCode != http.StatusOK {
				b.Fatalf("Unexpected status code: %d", resp.StatusCode)
			}

			_, err = io.Copy(io.Discard, resp.Body)
			if err != nil {
				b.Fatalf("Failed to read response: %v", err)
			}
			_ = resp.Body.Close()
		}
	})
}
