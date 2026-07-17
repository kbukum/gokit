package sample

import (
	"context"
	"fmt"
	"os"

	"github.com/kbukum/gokit/dataset/stage"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/fs"
	"github.com/kbukum/gokit/stream"
)

// realDir and aiDir are the subdirectories a [LocalTarget] writes items to by
// label.
const (
	realDir = "real"
	aiDir   = "ai"
)

// LocalTarget writes each item's payload to a real/ or ai/ subdirectory of an
// output directory, chosen by the item's label. Every destination is confined
// under the output directory through [fs] path safety, so a hostile item name
// cannot escape it.
type LocalTarget struct {
	name      string
	outputDir string
}

// NewLocalTarget returns a target that writes items under outputDir.
func NewLocalTarget(name, outputDir string) *LocalTarget {
	return &LocalTarget{name: name, outputDir: outputDir}
}

// Name returns the target's identifier.
func (t *LocalTarget) Name() string { return t.name }

// Publish writes every item to its label's subdirectory and reports the
// real/AI split.
func (t *LocalTarget) Publish(ctx context.Context, items *stream.Pipeline[Item]) (stage.PublishResult, error) {
	var realN, aiN int
	err := stream.ForEach(ctx, items, func(cbCtx context.Context, it Item) error {
		if err := cbCtx.Err(); err != nil {
			return err
		}
		sub := realDir
		if it.Label() == stage.LabelAI {
			sub = aiDir
		}
		if err := t.write(sub, it); err != nil {
			return err
		}
		if sub == aiDir {
			aiN++
		} else {
			realN++
		}
		return nil
	})
	if err != nil {
		return stage.PublishResult{}, err
	}
	return stage.PublishResult{
		TargetName:       t.name,
		Location:         t.outputDir,
		RecordsPublished: realN + aiN,
		Message:          fmt.Sprintf("real=%d ai=%d", realN, aiN),
	}, nil
}

// write streams one item's payload to a confined path under the label
// subdirectory, failing closed on a name that would escape the output dir.
func (t *LocalTarget) write(sub string, it Item) error {
	subDir, err := fs.SafeJoin(t.outputDir, sub)
	if err != nil {
		return err
	}
	dest, err := fs.SafeJoin(subDir, it.Name())
	if err != nil {
		return err
	}
	if merr := os.MkdirAll(subDir, 0o750); merr != nil {
		return apperrors.Internal(merr)
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return apperrors.Internal(err)
	}
	if _, werr := it.Payload().WriteTo(f); werr != nil {
		_ = f.Close()
		return werr
	}
	if err := f.Close(); err != nil {
		return apperrors.Internal(err)
	}
	return nil
}
