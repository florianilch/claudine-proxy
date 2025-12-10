//go:build goexperiment.jsonv2

package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	systemPrompt = "You are Claude Code, Anthropic's official CLI for Claude."
)

// normalizeJSON converts a JSON string to its canonical form for comparison.
// It removes whitespace differences to enable semantic equivalence testing.
func normalizeJSON(t *testing.T, s string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("Invalid JSON: %v\nJSON: %s", err, s)
	}
	normalized, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Failed to normalize JSON: %v", err)
	}
	return string(normalized)
}

// generateConversation creates realistic test JSON with specified number of messages.
// If systemPrompt is non-empty, includes a system field with that prompt.
func generateConversation(numMessages int, systemPrompt string) string {
	var sb strings.Builder
	sb.WriteString(`{"model": "claude-3-sonnet"`)

	if systemPrompt != "" {
		sb.WriteString(`, "system": [{"type": "text", "text": "`)
		sb.WriteString(systemPrompt)
		sb.WriteString(`"}]`)
	}

	sb.WriteString(`, "messages": [`)
	for i := range numMessages {
		if i > 0 {
			sb.WriteString(",")
		}
		// Realistic message size (~1KB per message)
		content := fmt.Sprintf("This is message %d. ", i) + strings.Repeat("The quick brown fox jumps over the lazy dog. ", 20)
		if i%2 == 0 {
			sb.WriteString(`{"role": "user", "content": "`)
		} else {
			sb.WriteString(`{"role": "assistant", "content": "`)
		}
		sb.WriteString(content)
		sb.WriteString(`"}`)
	}
	sb.WriteString(`], "temperature": 0.7, "max_tokens": 2000}`)
	return sb.String()
}

func TestSystemInjector(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "system key missing - append at end",
			input: `{
				"model": "claude-3-sonnet",
				"max_tokens": 1024,
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`,
			expected: `{
				"model": "claude-3-sonnet",
				"max_tokens": 1024,
				"messages": [
					{"role": "user", "content": "Hello"}
				],
				"system": [
					{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."}
				]
			}`,
		},
		{
			name: "system exists without element - inject as first element",
			input: `{
				"model": "claude-3-sonnet",
				"system": [
					{"type": "text", "text": "You are a helpful assistant."}
				],
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`,
			expected: `{
				"model": "claude-3-sonnet",
				"system": [
					{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."},
					{"type": "text", "text": "You are a helpful assistant."}
				],
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`,
		},
		{
			name: "system exists with element - pass through unchanged",
			input: `{
				"model": "claude-3-sonnet",
				"system": [
					{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."}
				],
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`,
			expected: `{
				"model": "claude-3-sonnet",
				"system": [
					{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."}
				],
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`,
		},
		{
			name: "system exists with element among others - prepend creating duplicate",
			input: `{
				"model": "claude-3-sonnet",
				"system": [
					{"type": "text", "text": "First instruction."},
					{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."},
					{"type": "text", "text": "Last instruction."}
				],
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`,
			expected: `{
				"model": "claude-3-sonnet",
				"system": [
					{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."},
					{"type": "text", "text": "First instruction."},
					{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."},
					{"type": "text", "text": "Last instruction."}
				],
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`,
		},
		{
			name:  "empty JSON object - add system",
			input: `{}`,
			expected: `{
				"system": [
					{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."}
				]
			}`,
		},
		{
			name: "large messages array - should stream efficiently",
			input: `{
				"model": "claude-3-sonnet",
				"messages": [
					{"role": "user", "content": "Message 1"},
					{"role": "assistant", "content": "Reply 1"},
					{"role": "user", "content": "Message 2"},
					{"role": "assistant", "content": "Reply 2"},
					{"role": "user", "content": "Message 3"}
				],
				"temperature": 0.7
			}`,
			expected: `{
				"model": "claude-3-sonnet",
				"messages": [
					{"role": "user", "content": "Message 1"},
					{"role": "assistant", "content": "Reply 1"},
					{"role": "user", "content": "Message 2"},
					{"role": "assistant", "content": "Reply 2"},
					{"role": "user", "content": "Message 3"}
				],
				"temperature": 0.7,
				"system": [
					{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."}
				]
			}`,
		},
		{
			name: "system key at end of object - process correctly",
			input: `{
				"model": "claude-3-sonnet",
				"messages": [],
				"system": [
					{"type": "text", "text": "Custom prompt"}
				]
			}`,
			expected: `{
				"model": "claude-3-sonnet",
				"messages": [],
				"system": [
					{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."},
					{"type": "text", "text": "Custom prompt"}
				]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create input reader and output buffer
			input := strings.NewReader(tt.input)
			output := &bytes.Buffer{}

			// Run transformation
			err := injectSystemPrompt(input, output)
			if err != nil {
				t.Fatalf("Transform failed: %v", err)
			}

			// Compare output
			got := normalizeJSON(t, output.String())
			want := normalizeJSON(t, tt.expected)

			if got != want {
				t.Errorf("Transformation mismatch:\ngot:  %s\nwant: %s", got, want)
			}
		})
	}
}

func TestSystemInjectorStreaming(t *testing.T) {
	// Test that transformer handles large requests efficiently
	t.Run("large request processing", func(t *testing.T) {
		// Create a large JSON
		var input strings.Builder
		input.WriteString(`{"model": "claude-3", "messages": [`)

		// Add many messages to simulate large request
		for i := range 1000 {
			if i > 0 {
				input.WriteString(",")
			}
			input.WriteString(`{"role": "user", "content": "`)
			input.WriteString(strings.Repeat("x", 1000)) // 1KB per message
			input.WriteString(`"}`)
		}
		input.WriteString(`]}`)

		reader := strings.NewReader(input.String())
		output := &bytes.Buffer{}

		err := injectSystemPrompt(reader, output)
		if err != nil {
			t.Fatalf("Transform failed: %v", err)
		}

		// Verify system was added
		var result map[string]any
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatalf("Invalid output JSON: %v", err)
		}

		if _, exists := result["system"]; !exists {
			t.Error("System field was not added to large request")
		}
	})
}

func TestSystemInjectorEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{
			name:      "invalid JSON - missing closing brace",
			input:     `{"model": "claude-3"`,
			expectErr: true,
		},
		{
			name:      "invalid JSON - malformed",
			input:     `{"model": invalid}`,
			expectErr: true,
		},
		{
			name:      "system key with non-array value - should handle gracefully",
			input:     `{"system": "not an array", "model": "claude-3"}`,
			expectErr: false,
		},
		{
			name:      "nested system keys - only transform top level",
			input:     `{"model": "claude-3", "config": {"system": []}}`,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.NewReader(tt.input)
			output := &bytes.Buffer{}

			err := injectSystemPrompt(input, output)

			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestImpersonationTransport(t *testing.T) {
	// Create test server that captures request headers and body
	var receivedBody string
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"test"}`))
	}))
	defer server.Close()

	// Create transport and client
	transport := &ImpersonationTransport{Base: http.DefaultTransport}
	client := &http.Client{Transport: transport}

	// Test: Request without system field should have it injected
	reqBody := `{"model":"claude-3","messages":[{"role":"user","content":"Hi"}]}`
	resp, err := client.Post(server.URL, "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()

	// Note: Transfer-Encoding header is processed by HTTP stack and not visible to server.
	// If the complete, valid JSON body is received, chunked encoding worked correctly.
	// We verify Content-Length is not set (signals chunked mode).
	if receivedHeaders.Get("Content-Length") != "" {
		t.Errorf("Content-Length header should not be set for chunked encoding, got: %s", receivedHeaders.Get("Content-Length"))
	}

	// Verify received JSON has correct system prompt
	var result map[string]any
	if err := json.Unmarshal([]byte(receivedBody), &result); err != nil {
		t.Fatalf("received invalid JSON: %v", err)
	}

	systemField, exists := result["system"]
	if !exists {
		t.Fatal("system field not injected")
	}

	// Verify exact system prompt content
	systemArray, ok := systemField.([]any)
	if !ok || len(systemArray) == 0 {
		t.Fatal("system field is not an array or is empty")
	}

	firstElem, ok := systemArray[0].(map[string]any)
	if !ok {
		t.Fatal("first system element is not an object")
	}

	if firstElem["type"] != "text" {
		t.Errorf("expected type 'text', got: %v", firstElem["type"])
	}

	expectedPrompt := "You are Claude Code, Anthropic's official CLI for Claude."
	if firstElem["text"] != expectedPrompt {
		t.Errorf("expected prompt %q, got: %v", expectedPrompt, firstElem["text"])
	}

	// Verify other fields preserved
	if result["model"] != "claude-3" {
		t.Errorf("model field not preserved correctly")
	}
}

func TestImpersonationTransportHeaderFiltering(t *testing.T) {
	// Create test server that captures request headers
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"test"}`))
	}))
	defer server.Close()

	transport := &ImpersonationTransport{Base: http.DefaultTransport}
	client := &http.Client{Transport: transport}

	// Create request with various headers
	req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{"model":"claude-3"}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// Set headers that should be filtered
	req.Header.Set("User-Agent", "custom-agent/1.0")
	req.Header.Set("X-Custom-Header", "should-be-filtered")
	req.Header.Set("X-Api-Key", "secret-key")

	// Set headers that should pass through
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token123")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()

	// Verify custom headers were filtered
	if receivedHeaders.Get("X-Custom-Header") != "" {
		t.Errorf("X-Custom-Header should be filtered, got: %s", receivedHeaders.Get("X-Custom-Header"))
	}
	if receivedHeaders.Get("X-Api-Key") != "" {
		t.Errorf("X-Api-Key should be filtered, got: %s", receivedHeaders.Get("X-Api-Key"))
	}

	// Verify allowed headers passed through
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type should pass through, got: %s", receivedHeaders.Get("Content-Type"))
	}
	if receivedHeaders.Get("Authorization") != "Bearer token123" {
		t.Errorf("Authorization should pass through, got: %s", receivedHeaders.Get("Authorization"))
	}

	// Verify Anthropic headers were set (impersonation)
	if receivedHeaders.Get("Anthropic-Version") != "2023-06-01" {
		t.Errorf("Anthropic-Version not set correctly, got: %s", receivedHeaders.Get("Anthropic-Version"))
	}
	if !strings.Contains(receivedHeaders.Get("Anthropic-Beta"), "oauth-2025-04-20") {
		t.Errorf("Anthropic-Beta not set correctly, got: %s", receivedHeaders.Get("Anthropic-Beta"))
	}
}

func TestImpersonationTransportFeatureMerging(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("X-Received-Beta", r.Header.Get("Anthropic-Beta"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"test"}`))
	}))
	defer server.Close()

	transport := &ImpersonationTransport{Base: http.DefaultTransport}
	client := &http.Client{Transport: transport}

	tests := []struct {
		name         string
		incomingBeta string
		want         string
	}{
		{
			name:         "no incoming beta - only required",
			incomingBeta: "",
			want:         "claude-code-20250219,oauth-2025-04-20",
		},
		{
			name:         "incoming beta - merged with required",
			incomingBeta: "fine-grained-tool-streaming-2025-05-14",
			want:         "claude-code-20250219,oauth-2025-04-20,fine-grained-tool-streaming-2025-05-14",
		},
		{
			name:         "multiple incoming features - preserved in order",
			incomingBeta: "fine-grained-tool-streaming-2025-05-14,interleaved-thinking-2025-05-14",
			want:         "claude-code-20250219,oauth-2025-04-20,fine-grained-tool-streaming-2025-05-14,interleaved-thinking-2025-05-14",
		},
		{
			name:         "incoming duplicate - deduplicated",
			incomingBeta: "oauth-2025-04-20,fine-grained-tool-streaming-2025-05-14,interleaved-thinking-2025-05-14",
			want:         "claude-code-20250219,oauth-2025-04-20,fine-grained-tool-streaming-2025-05-14,interleaved-thinking-2025-05-14",
		},
		{
			name:         "incoming with whitespace - trimmed",
			incomingBeta: " custom-beta , another-beta ",
			want:         "claude-code-20250219,oauth-2025-04-20,custom-beta,another-beta",
		},
		{
			name:         "very long feature name - buffer handling",
			incomingBeta: "very-long-feature-name-that-exceeds-normal-length-buffer-1234",
			want:         "claude-code-20250219,oauth-2025-04-20,very-long-feature-name-that-exceeds-normal-length-buffer-1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{"model":"claude-3"}`))
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			if tt.incomingBeta != "" {
				req.Header.Set("Anthropic-Beta", tt.incomingBeta)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			_ = resp.Body.Close()

			got := resp.Header.Get("X-Received-Beta")
			if got != tt.want {
				t.Errorf("Anthropic-Beta = %q, want %q", got, tt.want)
			}
		})
	}
}

func BenchmarkSystemInjector(b *testing.B) {
	inputs := []struct {
		name string
		json string
	}{
		{
			name: "tiny_100B",
			json: `{"model": "claude-3", "messages": [{"role": "user", "content": "Hi"}]}`,
		},
		// Missing system field - inject at end
		{
			name: "small_1KB_missing",
			json: generateConversation(1, ""),
		},
		{
			name: "medium_10KB_missing",
			json: generateConversation(10, ""),
		},
		{
			name: "large_100KB_missing",
			json: generateConversation(100, ""),
		},
		{
			name: "xlarge_1MB_missing",
			json: generateConversation(1000, ""),
		},
		// Has different system prompt - prepend to existing array
		{
			name: "small_1KB_inject",
			json: generateConversation(1, "You are a helpful assistant."),
		},
		{
			name: "medium_10KB_inject",
			json: generateConversation(10, "You are a helpful assistant."),
		},
		{
			name: "large_100KB_inject",
			json: generateConversation(100, "You are a helpful assistant."),
		},
		{
			name: "xlarge_1MB_inject",
			json: generateConversation(1000, "You are a helpful assistant."),
		},
		// Already has prompt - no modification
		{
			name: "small_1KB_noop",
			json: generateConversation(1, systemPrompt),
		},
		{
			name: "medium_10KB_noop",
			json: generateConversation(10, systemPrompt),
		},
		{
			name: "large_100KB_noop",
			json: generateConversation(100, systemPrompt),
		},
		{
			name: "xlarge_1MB_noop",
			json: generateConversation(1000, systemPrompt),
		},
	}

	for _, input := range inputs {
		actualSize := len(input.json)

		b.Run(fmt.Sprintf("%s_%dB", input.name, actualSize), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(actualSize))

			for b.Loop() {
				reader := strings.NewReader(input.json)
				output := &bytes.Buffer{}
				output.Grow(actualSize + 200) // Pre-allocate to avoid reallocs

				if err := injectSystemPrompt(reader, output); err != nil {
					b.Fatalf("Transform failed: %v", err)
				}
			}
		})
	}
}

func BenchmarkBuildBetaHeader(b *testing.B) {
	tests := []struct {
		name         string
		incomingBeta string
	}{
		{
			name:         "no_incoming",
			incomingBeta: "",
		},
		{
			name:         "single_new_feature",
			incomingBeta: "custom-feature-2025-01-01",
		},
		{
			name:         "multiple_new_features",
			incomingBeta: "feature-one,feature-two,feature-three",
		},
		{
			name:         "duplicate_required_feature",
			incomingBeta: "oauth-2025-04-20",
		},
		{
			name:         "mixed_duplicates_and_new",
			incomingBeta: "oauth-2025-04-20,custom-beta,claude-code-20250219,another-feature",
		},
		{
			name:         "with_whitespace",
			incomingBeta: " feature-one , feature-two , feature-three ",
		},
		{
			name:         "long_feature_names",
			incomingBeta: "very-long-feature-name-that-tests-buffer-allocation-1234567890,another-long-feature-name-for-testing",
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				_ = buildBetaHeader(tt.incomingBeta)
			}
		})
	}
}

func BenchmarkImpersonationTransport(b *testing.B) {
	// Setup minimal test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"test","type":"message"}`))
	}))
	defer server.Close()

	// Test cases with varying request sizes
	inputs := []struct {
		name string
		json string
	}{
		{
			name: "tiny_100B",
			json: `{"model": "claude-3", "messages": [{"role": "user", "content": "Hi"}]}`,
		},
		{
			name: "small_1KB",
			json: generateConversation(1, ""),
		},
		{
			name: "medium_10KB",
			json: generateConversation(10, ""),
		},
		{
			name: "large_100KB",
			json: generateConversation(100, ""),
		},
	}

	for _, input := range inputs {
		actualSize := len(input.json)

		b.Run(fmt.Sprintf("%s_%dB", input.name, actualSize), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(actualSize))

			transport := &ImpersonationTransport{Base: http.DefaultTransport}
			client := &http.Client{Transport: transport}

			for b.Loop() {
				resp, err := client.Post(server.URL, "application/json", strings.NewReader(input.json))
				if err != nil {
					b.Fatalf("request failed: %v", err)
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
		})
	}
}
