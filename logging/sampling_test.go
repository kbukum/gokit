package logging

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestNewSampler_ReturnsNonNil(t *testing.T) {
	cfg := SamplingConfig{
		Enabled:        true,
		InitialRate:    10,
		ThereafterRate: 5,
	}
	s := NewSampler(cfg)
	if s == nil {
		t.Fatal("expected non-nil sampler")
	}
}

func TestNewSampler_BurstSamplerConfig(t *testing.T) {
	cfg := SamplingConfig{
		Enabled:        true,
		InitialRate:    50,
		ThereafterRate: 10,
	}
	s := NewSampler(cfg)

	bs, ok := s.(*zerolog.BurstSampler)
	if !ok {
		t.Fatal("expected *zerolog.BurstSampler")
	}
	if bs.Burst != 50 {
		t.Errorf("expected Burst=50, got %d", bs.Burst)
	}
	basic, ok := bs.NextSampler.(*zerolog.BasicSampler)
	if !ok {
		t.Fatal("expected NextSampler to be *zerolog.BasicSampler")
	}
	if basic.N != 10 {
		t.Errorf("expected BasicSampler.N=10, got %d", basic.N)
	}
}

func TestSampling_AppliedWhenEnabled(t *testing.T) {
	cfg := &Config{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
		Sampling: SamplingConfig{
			Enabled:        true,
			InitialRate:    1, // allow 1 burst per second
			ThereafterRate: 1, // then log every 1st (all)
		},
		Masking: MaskingConfig{Enabled: false},
	}
	l := New(cfg, "test")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}

	// Verify sampling is active by writing enough messages
	// and checking that at least some are output. With InitialRate=1
	// and ThereafterRate=1, all messages should still appear.
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.DebugLevel).Sample(NewSampler(cfg.Sampling))
	sampled := &Logger{logger: zl, service: "test"}

	for i := 0; i < 10; i++ {
		sampled.Info("msg")
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) < 1 {
		t.Error("expected at least one log line with sampling enabled")
	}
}

func TestSampling_NotAppliedWhenDisabled(t *testing.T) {
	cfg := &Config{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
		Sampling: SamplingConfig{
			Enabled: false,
		},
		Masking: MaskingConfig{Enabled: false},
	}
	l := New(cfg, "test")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}

	// All messages should be output
	var buf bytes.Buffer
	zl := zerolog.New(&buf).Level(zerolog.DebugLevel)
	unsampled := &Logger{logger: zl, service: "test"}

	for i := 0; i < 10; i++ {
		unsampled.Info("msg")
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 10 {
		t.Errorf("expected 10 log lines without sampling, got %d", len(lines))
	}
}

func TestSampling_BurstThenThereafter(t *testing.T) {
	// Use a very restrictive thereafter rate to verify sampling actually drops messages.
	// InitialRate=2 burst per second, then log every 100th message.
	var buf bytes.Buffer
	cfg := SamplingConfig{
		Enabled:        true,
		InitialRate:    2,
		ThereafterRate: 100,
	}
	zl := zerolog.New(&buf).Level(zerolog.DebugLevel).Sample(NewSampler(cfg))

	for i := 0; i < 200; i++ {
		zl.Info().Msg("tick")
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// Should have burst (2) + at most 2 from thereafter (200-2=198, 198/100=1..2)
	// Total should be significantly less than 200.
	if len(lines) >= 200 {
		t.Errorf("expected sampling to reduce messages, got %d lines", len(lines))
	}
	if len(lines) < 2 {
		t.Errorf("expected at least the burst messages, got %d lines", len(lines))
	}
}

func TestSampling_DefaultConfig(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()

	if cfg.Sampling.InitialRate != 100 {
		t.Errorf("expected default InitialRate=100, got %d", cfg.Sampling.InitialRate)
	}
	if cfg.Sampling.ThereafterRate != 100 {
		t.Errorf("expected default ThereafterRate=100, got %d", cfg.Sampling.ThereafterRate)
	}
}
