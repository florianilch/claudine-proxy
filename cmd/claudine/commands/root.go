package commands

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/florianilch/claudine-proxy/internal/app"
	"github.com/florianilch/claudine-proxy/internal/observability"
	"github.com/urfave/cli/v3"
)

// Execute runs the root command with the given context and arguments.
func Execute(ctx context.Context, args []string) error {
	cmd := &cli.Command{
		Name:  "claudine",
		Usage: "Anthropic OAuth Ambassador",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "log-level",
				Usage: "log level (debug|info|warn|error)",
				Value: slog.LevelInfo.String(),
			},
		},
		Commands: []*cli.Command{
			proxyStartCommand(),
		},
	}

	return cmd.Run(ctx, args)
}

func proxyStartCommand() *cli.Command {
	return &cli.Command{
		Name:  "start",
		Usage: "Starts the proxy",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "log-format",
				Usage: "log format (text|json)",
				Value: "text",
			},
		},
		Action: proxyStartAction,
	}
}

func proxyStartAction(ctx context.Context, cmd *cli.Command) error {
	var level slog.Level
	err := level.UnmarshalText([]byte(cmd.String("log-level")))
	if err != nil {
		return err
	}

	// Set up observability before creating app
	err = observability.Instrument(level, cmd.String("log-format"))
	if err != nil {
		return fmt.Errorf("failed to set up observability layer: %w", err)
	}

	application, err := app.New()
	if err != nil {
		return fmt.Errorf("failed to create app: %w", err)
	}

	slog.InfoContext(ctx, "starting")

	if err := application.Start(ctx); err != nil {
		return fmt.Errorf("app failed to start: %w", err)
	}

	slog.InfoContext(ctx, "stopped gracefully")
	return nil
}
