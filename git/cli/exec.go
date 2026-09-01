package cli

import (
	"context"
	"fmt"
	"strings"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/redact"
	"github.com/kbukum/gokit/process"
)

// Exec runs the git CLI with the provided arguments in the repository root.
func (b *Backend) Exec(args ...string) ([]byte, error) {
	return b.run(context.Background(), args...)
}

func (b *Backend) run(ctx context.Context, args ...string) ([]byte, error) {
	result, err := b.runResult(ctx, args...)
	if err != nil {
		return nil, err
	}
	if result.ExitCodeOr(-1) != 0 {
		return result.Stdout, giterr.Internal(b.commandError(args, result))
	}
	return result.Stdout, nil
}

func (b *Backend) runResult(ctx context.Context, args ...string) (*process.Result, error) {
	result, err := process.Run(ctx, process.Command{
		Binary: b.executable,
		Args:   append(append([]string(nil), b.extraArgs...), args...),
		Dir:    b.root,
	})
	if err != nil && result.ExitCodeOr(-1) < 0 {
		return nil, giterr.Internal(err)
	}
	return result, nil
}

func (b *Backend) commandError(args []string, result *process.Result) error {
	msg := redact.URLCredentials(strings.TrimSpace(string(result.Stderr)))
	if msg == "" {
		msg = fmt.Sprintf("git exited with code %d", result.ExitCodeOr(-1))
	}
	full := append(append([]string(nil), b.extraArgs...), args...)
	sanitized := make([]string, len(full))
	for i, arg := range full {
		sanitized[i] = redactArg(arg)
	}
	return fmt.Errorf("git %v: %s", sanitized, msg)
}

// sensitiveArgKeys are substrings of a `key=value` argument key whose value carries
// credential material (e.g. `git -c http.extraHeader=Authorization: Basic ...`).
var sensitiveArgKeys = []string{"extraheader", "authorization", "password", "token", "secret"}

// redactArg masks credentials in a single CLI argument. It redacts URL credentials
// and the value of any `key=value` argument whose key names a secret, so raw extra
// args do not leak auth headers or tokens into error messages.
func redactArg(arg string) string {
	arg = redact.URLCredentials(arg)
	if key, value, ok := strings.Cut(arg, "="); ok && value != "" {
		lower := strings.ToLower(key)
		for _, name := range sensitiveArgKeys {
			if strings.Contains(lower, name) {
				return key + "=***"
			}
		}
	}
	return arg
}
