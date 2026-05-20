package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"mediaproxy/internal/agent"
)

func main() {
	configPath := flag.String("config", "/etc/node-agent/config.yaml", "path to YAML config")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := agent.LoadConfig(*configPath)
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	a := agent.New(cfg)
	if err := a.Run(ctx); err != nil && err != context.Canceled {
		slog.Error("agent stopped", "err", err)
		os.Exit(1)
	}
}
