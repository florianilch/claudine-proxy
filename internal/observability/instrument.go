package observability

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/processors/minsev"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otelStdoutlog "go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	otelGlobal "go.opentelemetry.io/otel/log/global"
	otelPropagation "go.opentelemetry.io/otel/propagation"
	otelSdkLog "go.opentelemetry.io/otel/sdk/log"
)

const (
	// ScopeName identifies this application's telemetry across all signals.
	ScopeName = "github.com/florianilch/claudine-proxy"
)

func Instrument(ctx context.Context, level slog.Level, logFormat string) (func(shutdownCtx context.Context) error, error) {
	var shutdownFuncs []func(context.Context) error
	var err error

	shutdown := func(shutdownCtx context.Context) error {
		var err error
		for _, shutdownFunc := range shutdownFuncs {
			shutdownErr := shutdownFunc(shutdownCtx)
			err = errors.Join(err, shutdownErr)
		}
		shutdownFuncs = nil
		return err
	}

	propagator := newPropagator()
	otel.SetTextMapPropagator(propagator)

	loggerProvider, err := newLoggerProvider(ctx, level)
	if err != nil {
		shutdownErr := shutdown(ctx)
		return shutdown, errors.Join(err, shutdownErr)
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	otelGlobal.SetLoggerProvider(loggerProvider)

	// Severity filtering happens at different layers:
	// stdout → slog.HandlerOptions.Level
	// OTel → minsev.Processor (implements FilterProcessor)
	logsExporter := os.Getenv("OTEL_LOGS_EXPORTER")
	var handler slog.Handler
	if logsExporter == "" || logsExporter == "none" {
		handler, err = newStdoutHandler(level, logFormat)
		if err != nil {
			shutdownErr := shutdown(ctx)
			return shutdown, errors.Join(err, shutdownErr)
		}
	} else {
		handler = otelslog.NewHandler(ScopeName)
	}
	slog.SetDefault(slog.New(handler))

	return shutdown, nil
}

// newStdoutHandler creates a handler for human-readable logs with trace correlation.
func newStdoutHandler(level slog.Level, logFormat string) (slog.Handler, error) {
	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	switch strings.ToLower(logFormat) {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		return nil, fmt.Errorf("unsupported log format %q (expected: json, text)", logFormat)
	}

	// traceContextHandler adds trace_id/span_id when spans are active
	return newTraceContextHandler(handler), nil
}

// newLoggerProvider creates a LoggerProvider configured by OTEL_LOGS_EXPORTER env var.
// Returns a no-op provider if unset or "none".
func newLoggerProvider(ctx context.Context, level slog.Level) (*otelSdkLog.LoggerProvider, error) {
	exporterType := os.Getenv("OTEL_LOGS_EXPORTER")

	if exporterType == "" || strings.ToLower(exporterType) == "none" {
		return otelSdkLog.NewLoggerProvider(), nil
	}

	var exporter otelSdkLog.Exporter
	var err error

	switch strings.ToLower(exporterType) {
	case "console":
		exporter, err = otelStdoutlog.New(otelStdoutlog.WithPrettyPrint())
	case "otlp":
		// Use OTEL_EXPORTER_OTLP_PROTOCOL to determine transport (default: http/protobuf per spec)
		// https://github.com/open-telemetry/opentelemetry-specification/blob/7f6d35f758bb5d92e354460d040974665a29ba32/specification/protocol/exporter.md
		protocol := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
		if protocol == "" {
			protocol = os.Getenv("OTEL_EXPORTER_OTLP_LOGS_PROTOCOL")
		}
		switch strings.ToLower(protocol) {
		case "grpc":
			exporter, err = otlploggrpc.New(ctx)
		case "http/protobuf", "":
			exporter, err = otlploghttp.New(ctx)
		default:
			return nil, fmt.Errorf("unsupported OTEL_EXPORTER_OTLP_PROTOCOL %q (expected: grpc, http/protobuf)", protocol)
		}
	default:
		return nil, fmt.Errorf("unsupported OTEL_LOGS_EXPORTER %q (expected: none, console, otlp)", exporterType)
	}

	if err != nil {
		return nil, err
	}

	// Use SimpleProcessor in tests for synchronous log export
	var processor otelSdkLog.Processor
	if flag.Lookup("test.v") != nil {
		processor = otelSdkLog.NewSimpleProcessor(exporter)
	} else {
		processor = otelSdkLog.NewBatchProcessor(exporter)
	}

	// minsev implements FilterProcessor for SDK-level severity filtering.
	// Direct cast works because slog.Level and minsev.Severity are numerically identical.
	processor = minsev.NewLogProcessor(processor, minsev.Severity(level))

	return otelSdkLog.NewLoggerProvider(
		otelSdkLog.WithProcessor(processor),
	), nil
}

func newPropagator() otelPropagation.TextMapPropagator {
	return otelPropagation.NewCompositeTextMapPropagator(
		otelPropagation.TraceContext{}, // W3C Trace Context (Traceparent/Tracestate)
	)
}
