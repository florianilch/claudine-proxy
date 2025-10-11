package observability

import (
	"log/slog"
	"os"
)

func Instrument() {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})
	slog.SetDefault(slog.New(handler))
}
