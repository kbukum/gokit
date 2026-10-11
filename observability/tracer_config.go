package observability

import (
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

func normalizedTracerConfig(config *TracerConfig) (TracerConfig, error) {
	if config == nil {
		return TracerConfig{}, apperrors.Validation("tracing configuration is required")
	}
	cfg := *config
	if err := cfg.Protocol.Validate(); err != nil {
		return TracerConfig{}, err
	}
	if strings.TrimSpace(cfg.ServiceName) == "" {
		return TracerConfig{}, apperrors.Validation("tracing service name is required")
	}
	host, port, err := net.SplitHostPort(cfg.Endpoint)
	collector, urlErr := url.Parse("https://" + cfg.Endpoint)
	portNumber, portErr := strconv.Atoi(port)
	if err != nil || host == "" || strings.ContainsAny(host, "/@?# \t\r\n") ||
		portErr != nil || portNumber < 1 || portNumber > 65535 ||
		urlErr != nil || collector.User != nil || collector.Path != "" || collector.RawQuery != "" || collector.Fragment != "" {
		return TracerConfig{}, apperrors.Validation("tracing endpoint must be a host and valid port, without credentials or URL components")
	}
	if math.IsNaN(cfg.SampleRate) || cfg.SampleRate < 0 || cfg.SampleRate > 1 {
		return TracerConfig{}, apperrors.Validation("tracing sample rate must be between zero and one")
	}
	if cfg.ExportTimeout < 0 || cfg.FlushTimeout < 0 || cfg.BatchTimeout < 0 || cfg.QueueSize < 0 || cfg.BatchSize < 0 {
		return TracerConfig{}, apperrors.Validation("tracing limits must be positive")
	}
	if cfg.ExportTimeout == 0 {
		cfg.ExportTimeout = 2 * time.Second
	}
	if cfg.FlushTimeout == 0 {
		cfg.FlushTimeout = 5 * time.Second
	}
	if cfg.BatchTimeout == 0 {
		cfg.BatchTimeout = time.Second
	}
	if cfg.QueueSize == 0 {
		cfg.QueueSize = 512
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = min(64, cfg.QueueSize)
	}
	if cfg.BatchSize > cfg.QueueSize {
		return TracerConfig{}, apperrors.Validation("tracing batch size must not exceed queue size")
	}
	if cfg.QueueSize > 65536 {
		return TracerConfig{}, apperrors.Validation("tracing queue size must not exceed 65536 spans")
	}
	return cfg, nil
}
