package llm

import (
	"fmt"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/llm/internal/streamwire"
)

// ErrStreamLimit identifies a rejected frame, output, argument or tool-count overflow.
var ErrStreamLimit = apperrors.New(apperrors.ErrCodeExternalService, "model stream limit exceeded")

// StreamLimits bounds cumulative untrusted model output before assembly grows. Zero selects defaults.
type StreamLimits struct {
	MaxOutputBytes   int `json:"max_output_bytes" yaml:"max_output_bytes"`
	MaxToolArgsBytes int `json:"max_tool_args_bytes" yaml:"max_tool_args_bytes"`
	MaxTools         int `json:"max_tools" yaml:"max_tools"`
}

func defaultStreamLimits() StreamLimits {
	return StreamLimits{MaxOutputBytes: 8 << 20, MaxToolArgsBytes: streamwire.MaxToolArgsBytes, MaxTools: 128}
}

func (l *StreamLimits) applyDefaults() {
	d := defaultStreamLimits()
	if l.MaxOutputBytes == 0 {
		l.MaxOutputBytes = d.MaxOutputBytes
	}
	if l.MaxToolArgsBytes == 0 {
		l.MaxToolArgsBytes = d.MaxToolArgsBytes
	}
	if l.MaxTools == 0 {
		l.MaxTools = d.MaxTools
	}
}

func (l StreamLimits) validate() error {
	if l.MaxOutputBytes <= 0 || l.MaxToolArgsBytes <= 0 || l.MaxTools <= 0 {
		return apperrors.InvalidInput("stream_limits", "all stream limits must be positive")
	}
	return nil
}

func streamLimit(name string, n int) error {
	return apperrors.New(apperrors.ErrCodeExternalService, fmt.Sprintf("llm: stream %s exceeded %d", name, n)).WithCause(ErrStreamLimit)
}
