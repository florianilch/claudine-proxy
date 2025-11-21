package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// dataReplacer escapes newlines in SSE data fields to maintain protocol integrity.
// SSE protocol requires multi-line data to prefix each line with "data:".
var dataReplacer = strings.NewReplacer(
	"\n", "\ndata:",
	"\r", "\\r",
)

// commentReplacer escapes newlines in SSE comment fields to maintain protocol integrity.
// SSE protocol requires multi-line comments to prefix each line with ":".
var commentReplacer = strings.NewReplacer(
	"\n", "\n: ",
	"\r", "\\r",
)

// fieldReplacer escapes newlines in SSE field values.
// These fields should be single-line values.
var fieldReplacer = strings.NewReplacer(
	"\n", "\\n",
	"\r", "\\r",
)

// Pre-allocated byte slices for SSE formatting to eliminate allocations on every write.
var (
	sseDataPrefix    = []byte("data: ")
	sseCommentPrefix = []byte(": ")
	sseEventPrefix   = []byte("event: ")
	sseNewline       = []byte("\n")
	sseTerminator    = []byte("\n\n")
)

// SSEWriter wraps http.ResponseWriter with Server-Sent Events protocol methods.
// Handles JSON marshaling, event formatting, and flushing for streaming responses.
type SSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// NewSSEWriter validates flushing support and sets required SSE headers.
// Returns error if the ResponseWriter doesn't implement http.Flusher,
// which is required for streaming responses.
func NewSSEWriter(w http.ResponseWriter) (*SSEWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("ResponseWriter doesn't implement http.Flusher")
	}

	w.Header().Set("Content-Type", "text/event-stream;charset=utf-8")
	w.Header().Set("Connection", "keep-alive")

	// Allow caller to override Cache-Control for custom caching strategies
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-cache")
	}

	return &SSEWriter{w: w, flusher: flusher}, nil
}

// WriteData marshals v to JSON and writes it as an SSE data event.
// Flushes immediately for real-time delivery.
func (s *SSEWriter) WriteData(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	// Use direct Write() calls to avoid []byte→string conversion allocation
	if _, err := s.w.Write(sseDataPrefix); err != nil {
		return err
	}

	if _, err := s.w.Write(data); err != nil {
		return err
	}

	if _, err := s.w.Write(sseTerminator); err != nil {
		return err
	}

	s.flusher.Flush()
	return nil
}

// WriteEvent writes an SSE event type field.
// Must be followed by WriteData or manual terminator to dispatch the event.
func (s *SSEWriter) WriteEvent(eventType string) error {
	if _, err := s.w.Write(sseEventPrefix); err != nil {
		return err
	}

	if _, err := fieldReplacer.WriteString(s.w, eventType); err != nil {
		return err
	}

	if _, err := s.w.Write(sseNewline); err != nil {
		return err
	}

	return nil
}

// WriteComment writes an SSE comment line (begins with ':').
// Useful for errors, heartbeats, or debugging information.
// Comments are ignored by SSE clients but visible in network logs.
func (s *SSEWriter) WriteComment(comment string) error {
	if _, err := s.w.Write(sseCommentPrefix); err != nil {
		return err
	}

	if _, err := commentReplacer.WriteString(s.w, comment); err != nil {
		return err
	}

	if _, err := s.w.Write(sseTerminator); err != nil {
		return err
	}

	s.flusher.Flush()
	return nil
}

// WriteRaw writes raw string as SSE data event without JSON marshaling.
// Useful for protocol-specific markers or pre-formatted data.
// Flushes immediately for real-time delivery.
func (s *SSEWriter) WriteRaw(data string) error {
	if _, err := s.w.Write(sseDataPrefix); err != nil {
		return err
	}

	if _, err := dataReplacer.WriteString(s.w, data); err != nil {
		return err
	}

	if _, err := s.w.Write(sseTerminator); err != nil {
		return err
	}

	s.flusher.Flush()
	return nil
}
