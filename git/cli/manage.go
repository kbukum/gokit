package cli

import (
	"context"
	"strings"

	"github.com/kbukum/gokit/git/internal/model"
)

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
	cfg := &model.CleanOptions{Force: true}
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
		case line != "":
			cleaned = append(cleaned, line)
		}
	}
	return cleaned, nil
}
