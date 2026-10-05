package httpclient

import (
	"context"
	"fmt"
	"time"

	"github.com/kbukum/gokit/util"
)

// StreamConfig supplies finite transport and protocol budgets. Zero values select defaults; negative values are invalid.
type StreamConfig struct {
	ConnectTimeout       time.Duration `yaml:"connect_timeout" mapstructure:"connect_timeout" json:"connect_timeout"`
	HeaderTimeout        time.Duration `yaml:"header_timeout" mapstructure:"header_timeout" json:"header_timeout"`
	FirstProgressTimeout time.Duration `yaml:"first_progress_timeout" mapstructure:"first_progress_timeout" json:"first_progress_timeout"`
	IdleTimeout          time.Duration `yaml:"idle_timeout" mapstructure:"idle_timeout" json:"idle_timeout"`
	TotalTimeout         time.Duration `yaml:"total_timeout" mapstructure:"total_timeout" json:"total_timeout"`
	MaxFrameBytes        int           `yaml:"max_frame_bytes" mapstructure:"max_frame_bytes" json:"max_frame_bytes"`
}

// ApplyDefaults fills unset budgets without allowing an unlimited stream.
func (c *StreamConfig) ApplyDefaults() {
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = 10 * time.Second
	}
	if c.HeaderTimeout == 0 {
		c.HeaderTimeout = 30 * time.Second
	}
	if c.FirstProgressTimeout == 0 {
		c.FirstProgressTimeout = time.Minute
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = 30 * time.Second
	}
	if c.TotalTimeout == 0 {
		c.TotalTimeout = 5 * time.Minute
	}
	if c.MaxFrameBytes == 0 {
		c.MaxFrameBytes = 1 << 20
	}
}

// Validate rejects nonpositive limits after defaulting.
func (c StreamConfig) Validate() error {
	if c.ConnectTimeout <= 0 || c.HeaderTimeout <= 0 || c.FirstProgressTimeout <= 0 || c.IdleTimeout <= 0 || c.TotalTimeout <= 0 || c.MaxFrameBytes <= 0 {
		return NewValidationError("stream budgets and frame limit must be positive")
	}
	if c.MaxFrameBytes > int(^uint(0)>>1)-2 {
		return NewValidationError("stream frame limit is too large")
	}
	return nil
}

// WithStreamClock injects runtime scheduling for streaming budgets.
func WithStreamClock(clock util.TimerClock) Option {
	return func(a *Adapter) {
		if clock != nil {
			a.streamClock = clock
		}
	}
}

func streamTimeout(phase string) error {
	return NewTimeoutError(fmt.Errorf("stream %s budget exceeded: %w", phase, context.DeadlineExceeded))
}
