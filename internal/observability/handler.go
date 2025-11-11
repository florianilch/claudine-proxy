package observability

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// traceContextHandler enriches log records with OpenTelemetry trace correlation
// attributes (trace_id and span_id) to enable log-trace correlation in distributed
// systems.
type traceContextHandler struct {
	handler slog.Handler
}

// newTraceContextHandler creates a handler that adds trace context to log records.
func newTraceContextHandler(handler slog.Handler) *traceContextHandler {
	return &traceContextHandler{handler: handler}
}

// Enabled reports whether the handler handles records at the given level.
func (h *traceContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle enriches the log record with trace correlation attributes (trace_id and
// span_id) when trace context is available, enabling correlation between logs and
// distributed traces.
func (h *traceContextHandler) Handle(ctx context.Context, record slog.Record) error {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", spanCtx.TraceID().String()),
			slog.String("span_id", spanCtx.SpanID().String()),
		)
	}

	return h.handler.Handle(ctx, record)
}

// WithAttrs returns a new handler with additional attributes.
func (h *traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceContextHandler{handler: h.handler.WithAttrs(attrs)}
}

// WithGroup returns a new handler with the given group name.
func (h *traceContextHandler) WithGroup(name string) slog.Handler {
	return &traceContextHandler{handler: h.handler.WithGroup(name)}
}
