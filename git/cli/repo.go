package cli

import "github.com/kbukum/gokit/git/internal/model"

// Backend provides CLI-backed git operations.
type Backend struct {
	root       string
	executable string
	extraArgs  []string
}

// New constructs a CLI backend for root.
func New(root string, cfg *model.OpenOptions) *Backend {
	backend := &Backend{root: root, executable: "git"}
	if cfg == nil {
		return backend
	}
	if cfg.CLIPath != "" {
		backend.executable = cfg.CLIPath
	}
	backend.extraArgs = append([]string(nil), cfg.ExtraArgs...)
	return backend
}
