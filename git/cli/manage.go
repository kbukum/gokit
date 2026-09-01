package cli

import (
	"context"
	"strings"
	"time"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/model"
)

const signingTimeout = 2 * time.Minute

// CreateSignedTag creates a signed annotated tag through git tag -s.
func (b *Backend) CreateSignedTag(name, target, message string, opts model.SignOptions) error {
	if strings.TrimSpace(opts.Key) == "" {
		if opts.Key != "" {
			return giterr.InvalidArg("key", "signing key must not be blank")
		}
		if err := b.requireSigningKey(); err != nil {
			return err
		}
	}
	if opts.Format != model.SignFormatDefault && opts.Format.GitConfig() == "" {
		return giterr.InvalidArg("format", "unsupported signing format")
	}

	ctx, cancel := context.WithTimeout(context.Background(), signingTimeout)
	defer cancel()
	_, err := b.run(ctx, signedTagArgs(name, target, message, opts)...)
	return err
}

func (b *Backend) requireSigningKey() error {
	ctx, cancel := context.WithTimeout(context.Background(), signingTimeout)
	defer cancel()
	result, err := b.runResult(ctx, "config", "--get", "user.signingkey")
	if err != nil && result.ExitCodeOr(-1) < 0 {
		return err
	}
	if result == nil || result.ExitCodeOr(-1) == 1 || strings.TrimSpace(string(result.Stdout)) == "" {
		return giterr.SigningKeyMissing("user.signingkey")
	}
	if result.ExitCodeOr(-1) != 0 {
		return giterr.Internal(b.commandError([]string{"config", "--get", "user.signingkey"}, result))
	}
	return nil
}

func signedTagArgs(name, target, message string, opts model.SignOptions) []string {
	args := make([]string, 0, 10)
	if opts.Format != model.SignFormatDefault {
		args = append(args, "-c", "gpg.format="+opts.Format.GitConfig())
	}
	if opts.Key != "" {
		args = append(args, "-c", "user.signingkey="+opts.Key)
	}
	args = append(args, "tag", "-s", "-m", message, "--", name, target)
	return args
}

func (b *Backend) GC() error {
	_, err := b.run(context.Background(), "gc")
	return err
}

func (b *Backend) Prune() error {
	_, err := b.run(context.Background(), "prune")
	return err
}

func (b *Backend) Fsck() error {
	_, err := b.run(context.Background(), "fsck")
	return err
}

func (b *Backend) Clean(opts ...model.CleanOption) ([]string, error) {
	cfg := &model.CleanOptions{Force: false}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	args := []string{"clean"}
	if cfg.Directories {
		args = append(args, "-d")
	}
	if cfg.Ignored {
		args = append(args, "-x")
	}
	if cfg.Force {
		args = append(args, "-f")
	} else {
		args = append(args, "-n")
	}
	args = append(args, cfg.ExtraArgs...)

	out, err := b.run(context.Background(), args...)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return nil, nil
	}

	var cleaned []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Removing "):
			cleaned = append(cleaned, strings.TrimPrefix(line, "Removing "))
		case strings.HasPrefix(line, "Would remove "):
			cleaned = append(cleaned, strings.TrimPrefix(line, "Would remove "))
		default:
			continue
		}
	}
	return cleaned, nil
}
