package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/model"
)

func (b *Backend) Merge(revision string) error {
	_, err := b.run(context.Background(), "merge", revision)
	return err
}

func (b *Backend) MergeAbort() error {
	_, err := b.run(context.Background(), "merge", "--abort")
	return err
}

func (b *Backend) Rebase(onto string) error {
	_, err := b.run(context.Background(), "rebase", onto)
	return err
}

func (b *Backend) RebaseContinue() error {
	_, err := b.run(context.Background(), "rebase", "--continue")
	return err
}

func (b *Backend) RebaseAbort() error {
	_, err := b.run(context.Background(), "rebase", "--abort")
	return err
}

func (b *Backend) CherryPick(revision string) error {
	_, err := b.run(context.Background(), "cherry-pick", revision)
	return err
}

func (b *Backend) CherryPickContinue() error {
	_, err := b.run(context.Background(), "cherry-pick", "--continue")
	return err
}

func (b *Backend) CherryPickAbort() error {
	_, err := b.run(context.Background(), "cherry-pick", "--abort")
	return err
}

func (b *Backend) Reset(target string, mode model.ResetMode, paths ...string) error {
	args := []string{"reset", "--" + mode.String(), target}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	_, err := b.run(context.Background(), args...)
	return err
}

func (b *Backend) Checkout(target string, paths ...string) error {
	args := []string{"checkout", target}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	_, err := b.run(context.Background(), args...)
	return err
}

func (b *Backend) StashPush(message string) error {
	args := []string{"stash", "push"}
	if strings.TrimSpace(message) != "" {
		args = append(args, "-m", message)
	}
	_, err := b.run(context.Background(), args...)
	return err
}

func (b *Backend) StashPop(index int) error {
	_, err := b.run(context.Background(), "stash", "pop", fmt.Sprintf("stash@{%d}", index))
	return err
}

func (b *Backend) StashList() ([]model.StashEntry, error) {
	out, err := b.run(context.Background(), "stash", "list")
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return nil, nil
	}

	lines := strings.Split(text, "\n")
	entries := make([]model.StashEntry, 0, len(lines))
	for _, line := range lines {
		name, payload, ok := strings.Cut(line, ": ")
		if !ok || !strings.HasPrefix(name, "stash@{") || !strings.HasSuffix(name, "}") {
			return nil, giterr.Internal(fmt.Errorf("invalid git stash output: %q", line))
		}

		indexText := strings.TrimSuffix(strings.TrimPrefix(name, "stash@{"), "}")
		index, convErr := strconv.Atoi(indexText)
		if convErr != nil {
			return nil, giterr.Internal(fmt.Errorf("invalid stash index %q: %w", indexText, convErr))
		}

		commit, revErr := b.RevParse(name + "^1")
		if revErr != nil {
			return nil, revErr
		}

		entries = append(entries, model.StashEntry{
			Index:   index,
			Name:    name,
			Message: parseStashMessage(payload),
			Commit:  commit,
		})
	}
	return entries, nil
}

func parseStashMessage(payload string) string {
	message := payload
	if _, rest, ok := strings.Cut(payload, ": "); ok {
		message = rest
		if prefix, suffix, ok := strings.Cut(rest, " "); ok && isHexString(prefix) {
			message = suffix
		}
	}
	return message
}

func isHexString(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
