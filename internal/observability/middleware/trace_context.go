package middleware

import (
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// TraceContextExtraction extracts W3C trace context from Traceparent/Tracestate headers
// and adds trace_id/span_id to both httplog attributes and the request context.
//
// This enables distributed tracing participation without creating spans:
//   - Reads trace context from incoming request headers
//   - Stores it in request context for downstream logging
//   - Sets httplog attributes for immediate visibility
//
// Trace context flows: Client → Headers → Context → Logs → Observability backend
func TraceContextExtraction(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract trace context from Traceparent/Tracestate headers into context
		propagator := otel.GetTextMapPropagator()
		ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		// Read the SpanContext (works without active span)
		spanCtx := trace.SpanContextFromContext(ctx)
		if spanCtx.IsValid() {
			traceID := spanCtx.TraceID().String()
			spanID := spanCtx.SpanID().String()

			// Set attributes for Logging middleware to include in request logs.
			// SetLogAttrs is no-op if Logging middleware does not exist.
			SetLogAttrs(ctx,
				slog.String("trace_id", traceID),
				slog.String("span_id", spanID),
			)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
