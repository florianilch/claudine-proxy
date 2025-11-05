package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"

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
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "path to config file",
			},
			&cli.StringFlag{
				Name:  "log-level",
				Usage: "log level (debug|info|warn|error)",
				Value: slog.LevelInfo.String(),
			},
		},
		Commands: []*cli.Command{
			proxyStartCommand(),
			authCommand(),
		},
	}

	return cmd.Run(ctx, args)
}

func proxyStartCommand() *cli.Command {
	return &cli.Command{
		Name: "start",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "log-format",
				Usage: "log format (text|json)",
				Value: string(app.DefaultConfigLogFormat),
			},
			&cli.StringFlag{
				Name:  "server--host",
				Usage: "server host",
				Value: app.DefaultConfigServerHost,
			},
			&cli.IntFlag{
				Name:  "server--port",
				Usage: "server port",
				Value: int(app.DefaultConfigServerPort),
			},
			&cli.StringFlag{
				Name:  "upstream--base-url",
				Usage: "upstream API base URL",
				Value: app.DefaultConfigUpstreamBaseURL,
			},
		},
		Action: proxyStartAction,
	}
}

func proxyStartAction(ctx context.Context, cmd *cli.Command) error {
	cfg, err := loadConfig(cmd.String("config"), cmd, os.Environ)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Set up observability before creating app
	err = observability.Instrument(cfg.LogLevel, string(cfg.LogFormat))
	if err != nil {
		return fmt.Errorf("failed to set up observability layer: %w", err)
	}

	application, err := app.New(cfg)
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
