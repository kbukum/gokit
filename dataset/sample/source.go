package sample

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/kbukum/gokit/dataset/payload"
	"github.com/kbukum/gokit/dataset/stage"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/fs"
	"github.com/kbukum/gokit/stream"
)

// NewSliceSource returns a [stage.Source] over a fixed slice of items, for composition and tests.
func NewSliceSource(name string, items []Item) stage.Source[Item] {
	return stage.NewSliceSource(name, items)
}

// DirSource is a [stage.Source] over the regular files in a directory: each file becomes a file-backed [Item] tagged with the source's label, offset by its sorted position. File bytes are not read until a target consumes them; a file larger than the payload limit fails the source closed.
type DirSource struct {
	name   string
	dir    string
	label  stage.Label
	limits payload.Limits
}

// NewDirSource returns a directory-backed item source. Every produced item is tagged with label; the payloads are file-backed and confined to dir, and a file exceeding limits fails the source closed.
func NewDirSource(name, dir string, label stage.Label, limits payload.Limits) *DirSource {
	return &DirSource{name: name, dir: dir, label: label, limits: limits.WithDefaults()}
}

// Name returns the source's identifier.
func (s *DirSource) Name() string { return s.name }

// CacheKey fingerprints the source by directory.
func (s *DirSource) CacheKey() string { return "dir:" + s.dir }

// Stream lists the directory lazily, surfacing any listing or path-safety error on the first pull.
func (s *DirSource) Stream(context.Context) *stream.Pipeline[Item] {
	return stream.FromFunc(func(ctx context.Context) stream.Iterator[Item] {
		items, err := s.list()
		if err != nil {
			return &failIter{err: err}
		}
		return stream.FromSlice(items).Iter(ctx)
	})
}

// list builds the items for the directory's regular files in sorted order, failing closed on a file larger than the payload limit.
func (s *DirSource) list() ([]Item, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	items := make([]Item, 0, len(names))
	for i, name := range names {
		path, err := fs.SafeJoin(s.dir, name)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, apperrors.Internal(err)
		}
		if info.Size() > s.limits.MaxInMemoryBytes {
			return nil, apperrors.InvalidInput("payload",
				fmt.Sprintf("file %q of %d bytes exceeds payload limit of %d bytes",
					name, info.Size(), s.limits.MaxInMemoryBytes))
		}
		items = append(items, New(name, s.label, i, payload.FromFile(path)))
	}
	return items, nil
}

// failIter yields err on first pull, deferring a listing error into the stream.
type failIter struct{ err error }

func (it *failIter) Next(context.Context) (Item, bool, error) { return Item{}, false, it.err }
func (it *failIter) Close() error                             { return nil }
