package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/observability"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "broadcast-demo:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logger := logging.NewDefault("broadcast-demo").Slog()

	counter, err := observability.NewInt64Counter(
		"broadcast-demo",
		"config_change_dropped_total",
		observability.WithInstrumentDescription("config changes dropped because a subscriber lagged"),
	)
	if err != nil {
		return fmt.Errorf("create drop counter: %w", err)
	}

	cfg := RunConfig{
		Subscribers: 3,
		Buffer:      4,
		Events:      12,
		Source:      "config-file",
		DropCounter: counter,
	}

	logger.InfoContext(ctx, "starting broadcast fan-out",
		"subscribers", cfg.Subscribers, "buffer", cfg.Buffer, "events", cfg.Events)

	result, err := Run(ctx, cfg)
	if err != nil {
		return err
	}

	for i, n := range result.Healthy {
		logger.InfoContext(ctx, "healthy subscriber kept up",
			"subscriber", i, "received", n, "of", cfg.Events)
	}
	logger.WarnContext(ctx, "slow subscriber lagged and dropped overflow",
		"buffered", result.SlowReceived, "dropped", result.Dropped)

	fmt.Printf("\nBroadcast summary: %d healthy subscribers received %d events each; "+
		"the slow subscriber buffered %d and dropped %d.\n",
		cfg.Subscribers, cfg.Events, result.SlowReceived, result.Dropped)
	return nil
}
