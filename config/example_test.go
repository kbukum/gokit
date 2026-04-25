package config_test

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/kbukum/gokit/config"
)

// AppConfig is the strongly typed configuration struct your service uses.
type AppConfig struct {
	Service struct {
		Name string `mapstructure:"name"`
		Port int    `mapstructure:"port"`
	} `mapstructure:"service"`
}

// ExampleLoadConfig demonstrates loading configuration with a structured
// slog-compatible warning logger.
func ExampleLoadConfig() {
	warn := func(msg string, attrs ...slog.Attr) {
		slog.LogAttrs(nil, slog.LevelWarn, msg, attrs...) //nolint:staticcheck // example: SA1012
	}

	var cfg AppConfig
	err := config.LoadConfig("my-service", &cfg,
		config.WithProfile(""),         // no profile
		config.WithWarningLogger(warn), // structured warnings
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load failed:", err)
	}
	// Output:
}
