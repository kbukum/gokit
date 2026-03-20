package report

import (
	"io"

	"github.com/kbukum/gokit/bench"
)

// Reporter generates formatted output from benchmark results.
type Reporter interface {
	// Name returns the reporter's format name.
	Name() string
	// Generate writes the formatted report to w.
	Generate(w io.Writer, result *bench.RunResult) error
}
