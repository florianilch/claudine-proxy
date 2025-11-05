package observability

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

func Instrument(level slog.Level, logFormat string) error {
	handler, err := newStdoutHandler(level, logFormat)
	if err != nil {
		return err
	}

	slog.SetDefault(slog.New(handler))

	return nil
}

// newStdoutHandler creates a handler for human-readable logs.
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

	return handler, nil
}
