//go:build goexperiment.jsonv2

package proxy

import (
	"encoding/json"
	"encoding/json/jsontext"
	"io"
	"net/http"
)

const claudeCodeSystemPrompt = "You are Claude Code, Anthropic's official CLI for Claude."

var (
	systemPromptElement = mustMarshal(map[string]string{"type": "text", "text": claudeCodeSystemPrompt})
	systemPromptArray   = mustMarshal([]json.RawMessage{systemPromptElement})

	// allowedHeaders defines the HTTP headers permitted to pass through to the Anthropic API.
	allowedHeaders = map[string]bool{
		"Content-Type":    true,
		"Content-Length":  true,
		"Accept":          true,
		"Accept-Encoding": true,
		"Authorization":   true,
	}
)

// ImpersonationTransport is an http.RoundTripper that impersonates Claude Code.
type ImpersonationTransport struct {
	Base http.RoundTripper
}

// Compile-time check that ImpersonationTransport implements http.RoundTripper.
var _ http.RoundTripper = (*ImpersonationTransport)(nil)

// RoundTrip implements http.RoundTripper interface.
// Filters/sets headers and transforms the request body to inject system prompt.
func (t *ImpersonationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	// Clone request for modification
	newReq := req.Clone(req.Context())

	// Filter headers to prevent client-side headers (User-Agent, custom headers, etc.)
	// from breaking Anthropic API requirements or leaking proxy implementation details.
	originalHeaders := newReq.Header
	newReq.Header = make(http.Header)
	for key, values := range originalHeaders {
		if allowedHeaders[key] {
			newReq.Header[key] = values
		}
	}

	// Set required Anthropic API headers for impersonation
	newReq.Header.Set("Anthropic-Beta", "oauth-2025-04-20,claude-code-20250219,interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14")
	newReq.Header.Set("Anthropic-Version", "2023-06-01")

	// Skip body transformation for non-POST requests or requests without bodies
	if req.Method != http.MethodPost || req.Body == nil {
		return base.RoundTrip(newReq)
	}

	// Create pipe for streaming body transformation
	pr, pw := io.Pipe()

	// Start transformation in background goroutine.
	// Note: No goroutine leak on context cancellation. When http.Transport cancels
	// the request, it closes pr, which unblocks all writes to pw with ErrClosedPipe.
	go func() {
		err := injectSystemPrompt(req.Body, pw)
		// CloseWithError(nil) is equivalent to Close() but allows error handling
		pw.CloseWithError(err)
		_ = req.Body.Close()
	}()

	newReq.Body = pr

	// Streaming transformation changes body length, requiring chunked transfer encoding.
	// ContentLength = -1 enables chunked mode; remove Content-Length header to prevent conflicts.
	newReq.ContentLength = -1
	newReq.Header.Del("Content-Length")

	return base.RoundTrip(newReq)
}

// injectSystemPrompt uses encoding/json/jsontext for streaming JSON transformation.
//
// We need to inject a system prompt into API requests without buffering the entire
// request in memory. The standard library's json.Decoder.Token() strips formatting
// (quotes, commas, colons), making reconstruction complex and error-prone. Buffering
// with json.RawMessage is simple but uses excessive memory for large requests.
//
// The jsontext package (Go 1.25+ with GOEXPERIMENT=jsonv2) provides token-based
// streaming WITH preserved formatting, combining optimal performance with
// implementation simplicity.
//
// Uses mixed token/value streaming for optimal performance:
// - Token-level streaming: Efficient for fields we don't modify (most of the request)
// - Value-level handling: Only for "system" field which needs inspection/modification
//
// If "system" not found during object traversal, inject before closing brace.
func injectSystemPrompt(r io.Reader, w io.Writer) error {
	dec := jsontext.NewDecoder(r)
	enc := jsontext.NewEncoder(w)

	tok, err := dec.ReadToken()
	if err != nil {
		return err
	}
	if tok.Kind() != '{' {
		return enc.WriteToken(tok) // Not an object, pass through
	}
	if err := enc.WriteToken(tok); err != nil {
		return err
	}

	foundSystem := false

	for dec.PeekKind() != '}' {
		key, err := dec.ReadToken()
		if err != nil {
			return err
		}

		// Write key (encoder automatically adds : after it)
		if err := enc.WriteToken(key); err != nil {
			return err
		}

		// String tokens have Kind() == '"'
		if key.Kind() == '"' && key.String() == "system" {
			foundSystem = true

			systemVal, err := dec.ReadValue()
			if err != nil {
				return err
			}

			if err := ensureSystemPrompt(enc, systemVal); err != nil {
				return err
			}
		} else {
			// Stream other fields unchanged
			val, err := dec.ReadValue()
			if err != nil {
				return err
			}
			if err := enc.WriteValue(val); err != nil {
				return err
			}
		}
	}

	// Streaming constraint: inject system at end if not found during traversal
	if !foundSystem {
		if err := enc.WriteToken(jsontext.String("system")); err != nil {
			return err
		}
		if err := enc.WriteValue(jsontext.Value(systemPromptArray)); err != nil {
			return err
		}
	}

	tok, err = dec.ReadToken()
	if err != nil {
		return err
	}
	return enc.WriteToken(tok)
}

// ensureSystemPrompt checks if prompt is the first element and adds it if not.
// Writes directly to the encoder to avoid intermediate allocations.
func ensureSystemPrompt(enc *jsontext.Encoder, systemVal jsontext.Value) error {
	// Try to parse as array
	var systemArray []json.RawMessage
	if err := json.Unmarshal([]byte(systemVal), &systemArray); err != nil {
		// Not a valid array, replace with pre-marshaled array
		return enc.WriteValue(jsontext.Value(systemPromptArray))
	}

	// Check if empty
	if len(systemArray) == 0 {
		return enc.WriteValue(jsontext.Value(systemPromptArray))
	}

	// Check first element
	var firstElem map[string]string
	if err := json.Unmarshal(systemArray[0], &firstElem); err == nil {
		if firstElem["type"] == "text" && firstElem["text"] == claudeCodeSystemPrompt {
			// Already has prompt, return unchanged
			return enc.WriteValue(systemVal)
		}
	}

	// Need to prepend prompt - stream directly using pre-marshaled element
	if err := enc.WriteToken(jsontext.BeginArray); err != nil {
		return err
	}
	if err := enc.WriteValue(jsontext.Value(systemPromptElement)); err != nil {
		return err
	}
	for _, elem := range systemArray {
		if err := enc.WriteValue(jsontext.Value(elem)); err != nil {
			return err
		}
	}
	return enc.WriteToken(jsontext.EndArray)
}

// mustMarshal marshals the value to JSON or panics.
// Used for package-level initialization of system prompt constants.
func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic("failed to marshal system prompt: " + err.Error())
	}
	return data
}
