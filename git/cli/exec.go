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
	result, err := b.runResult(ctx, args...)
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return result.Stdout, giterr.Internal(commandError(args, result))
	}
	return result.Stdout, nil
}

func (b *Backend) runResult(ctx context.Context, args ...string) (*process.Result, error) {
	result, err := process.Run(ctx, process.Command{
		Binary: b.executable,
		Args:   append(append([]string(nil), b.extraArgs...), args...),
		Dir:    b.root,
	})
	if err != nil && (result == nil || result.ExitCode < 0) {
		return nil, giterr.Internal(err)
	}
	return result, nil
}

func commandError(args []string, result *process.Result) error {
	msg := redactCredentials(strings.TrimSpace(string(result.Stderr)))
	if msg == "" {
		msg = fmt.Sprintf("git exited with code %d", result.ExitCode)
	}
	sanitized := make([]string, len(args))
	for i, arg := range args {
		sanitized[i] = redactCredentials(arg)
	}
	return fmt.Errorf("git %v: %s", sanitized, msg)
}

// redactCredentials masks credentials in URLs to prevent leakage in
// error messages and logs. Handles all http(s)://user:pass@host occurrences.
func redactCredentials(s string) string {
	result := s
	for _, scheme := range []string{"https://", "http://"} {
		var offset int
		for {
			idx := strings.Index(result[offset:], scheme)
			if idx < 0 {
				break
			}
			idx += offset
			rest := result[idx+len(scheme):]
			atIdx := strings.Index(rest, "@")
			if atIdx < 0 {
				break
			}
			userinfo := rest[:atIdx]
			if user, _, ok := strings.Cut(userinfo, ":"); ok {
				redacted := scheme + user + ":***@"
				result = result[:idx] + redacted + rest[atIdx+1:]
				offset = idx + len(redacted)
			} else {
				offset = idx + len(scheme)
			}
		}
	}
	return result
}
