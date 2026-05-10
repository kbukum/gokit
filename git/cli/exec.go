package cli

import (
	"context"
	"fmt"
	"strings"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/process"
)

// Exec runs the git CLI with the provided arguments in the repository root.
func (b *Backend) Exec(args ...string) ([]byte, error) {
	return b.run(context.Background(), args...)
}

func (b *Backend) run(ctx context.Context, args ...string) ([]byte, error) {
	result, err := process.Run(ctx, process.Command{
		Binary: b.executable,
		Args:   append(append([]string(nil), b.extraArgs...), args...),
		Dir:    b.root,
	})
	if err != nil {
		return nil, giterr.Internal(err)
	}
	if result.ExitCode != 0 {
		msg := strings.TrimSpace(string(result.Stderr))
		if msg == "" {
			msg = fmt.Sprintf("git exited with code %d", result.ExitCode)
		}
		return result.Stdout, giterr.Internal(fmt.Errorf("git %v: %s", args, msg))
	}
	return result.Stdout, nil
}
