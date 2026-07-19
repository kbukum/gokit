package cli

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/model"
)

func (b *Backend) Describe(revision string) (string, error) {
	args := []string{"describe"}
	if revision != "" {
		args = append(args, revision)
	}
	out, err := b.run(context.Background(), args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (b *Backend) RevParse(spec string) (model.Oid, error) {
	out, err := b.run(context.Background(), "rev-parse", spec)
	if err != nil {
		return model.Oid{}, err
	}
	return parseOID(strings.TrimSpace(string(out)))
}

func (b *Backend) Grep(pattern string, paths ...string) ([]model.GrepMatch, error) {
	args := []string{"grep", "-n", "--", pattern}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	result, err := b.runResult(context.Background(), args...)
	if err != nil {
		return nil, err
	}
	if result.ExitCode == 1 {
		return nil, nil
	}
	if result.ExitCode != 0 {
		return nil, giterr.Internal(b.commandError(args, result))
	}
	out := result.Stdout
	text := strings.TrimSpace(string(out))
	if text == "" {
		return nil, nil
	}

	lines := strings.Split(text, "\n")
	matches := make([]model.GrepMatch, 0, len(lines))
	for _, line := range lines {
		path, rest, ok := strings.Cut(line, ":")
		if !ok {
			return nil, giterr.Internal(fmt.Errorf("invalid git grep output: %q", line))
		}
		lineText, content, ok := strings.Cut(rest, ":")
		if !ok {
			return nil, giterr.Internal(fmt.Errorf("invalid git grep output: %q", line))
		}
		lineNo, convErr := strconv.Atoi(lineText)
		if convErr != nil {
			return nil, giterr.Internal(fmt.Errorf("invalid git grep line number %q: %w", lineText, convErr))
		}
		matches = append(matches, model.GrepMatch{
			Path:    path,
			Line:    lineNo,
			Content: content,
		})
	}
	return matches, nil
}

func (b *Backend) Show(spec string) ([]byte, error) {
	return b.run(context.Background(), "show", spec)
}

func parseOID(text string) (model.Oid, error) {
	var oid model.Oid
	if len(text) != hex.EncodedLen(len(oid)) {
		return model.Oid{}, giterr.Internal(fmt.Errorf("invalid object id length %d", len(text)))
	}
	decoded, err := hex.DecodeString(text)
	if err != nil {
		return model.Oid{}, giterr.Internal(fmt.Errorf("invalid object id %q: %w", text, err))
	}
	copy(oid[:], decoded)
	return oid, nil
}
