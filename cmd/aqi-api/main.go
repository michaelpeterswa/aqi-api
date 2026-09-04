package main

import (
	"context"
	"log"
	"log/slog"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/michaelpeterswa/aqi-api/internal/config"
	"github.com/michaelpeterswa/aqi-api/internal/logging"
)

func main() {
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "error"
	}

	slogLevel, err := logging.LogLevelToSlogLevel(logLevel)
	if err != nil {
		log.Fatalf("could not convert log level: %s", err)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel,
	})))

	cmd := &cli.Command{
		Name:  "aqi-api",
		Usage: "serve and publish particulate and AQI data out of TimescaleDB",
		Commands: []*cli.Command{
			{
				Name:  "serve",
				Usage: "run the HTTP API",
				Action: func(ctx context.Context, _ *cli.Command) error {
					c, err := config.NewConfig()
					if err != nil {
						return err
					}
					return runServe(ctx, c)
				},
			},
			{
				Name:  "publish",
				Usage: "compute a snapshot and upload it to the public bucket",
				Action: func(ctx context.Context, _ *cli.Command) error {
					c, err := config.NewConfig()
					if err != nil {
						return err
					}
					return runPublish(ctx, c)
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		slog.Error("command failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
