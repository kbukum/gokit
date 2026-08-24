package embedded

import (
	"fmt"
	"strings"

	gogit "github.com/go-git/go-git/v5"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/model"
)

// Blame returns line-level attribution for a file at a revision.
func (b *Backend) Blame(revision, path string, opts ...model.BlameOption) ([]model.BlameLine, error) {
	cfg := model.BlameOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.EndLine > 0 && cfg.StartLine > cfg.EndLine {
		return nil, giterr.InvalidLineRange(cfg.StartLine, cfg.EndLine)
	}

	commit, err := b.commitForRef(revision)
	if err != nil {
		return nil, err
	}
	path = strings.ReplaceAll(path, "\\", "/")
	if _, fileErr := commit.File(path); fileErr != nil {
		return nil, giterr.RefNotFound(fmt.Sprintf("%s:%s", revision, path))
	}

	result, err := gogit.Blame(commit, path)
	if err != nil {
		return nil, giterr.Internal(err)
	}

	start, end, err := blameRange(len(result.Lines), cfg)
	if err != nil {
		return nil, err
	}

	lines := make([]model.BlameLine, 0, end-start)
	for idx := start; idx < end; idx++ {
		line := result.Lines[idx]
		lines = append(lines, model.BlameLine{
			Line:      idx + 1,
			CommitOID: oidFromHash(line.Hash),
			Author:    model.Signature{Name: line.AuthorName, Email: line.Author, When: line.Date},
			Content:   line.Text,
		})
	}
	return lines, nil
}

func blameRange(total int, opts model.BlameOptions) (start, end int, err error) {
	if opts.StartLine < 0 || opts.EndLine < 0 {
		return 0, 0, giterr.InvalidLineRange(opts.StartLine, opts.EndLine)
	}
	start = 1
	if opts.StartLine > 0 {
		start = opts.StartLine
	}
	end = total
	if opts.EndLine > 0 {
		end = opts.EndLine
	}
	if start < 1 || start > end {
		return 0, 0, giterr.InvalidLineRange(opts.StartLine, opts.EndLine)
	}
	if start > total {
		return total, total, nil
	}
	if end > total {
		end = total
	}
	return start - 1, end, nil
}
