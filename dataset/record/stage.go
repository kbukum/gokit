package record

import (
	"bytes"
	"context"
	"io"

	"github.com/kbukum/gokit/dataset/payload"
	"github.com/kbukum/gokit/dataset/stage"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/fs"
	"github.com/kbukum/gokit/stream"
)

// FileSource is a [stage.Source] that reads a tabular file in one of the supported [Format]s. Reading is deferred to stream time and bounded by limits, so the source composes with the generic collector. Its cache key fingerprints the format and path.
type FileSource struct {
	name   string
	path   string
	format Format
	limits payload.Limits
}

// NewFileSource returns a file-backed record source in the given format.
func NewFileSource(name, path string, format Format, limits payload.Limits) *FileSource {
	return &FileSource{name: name, path: path, format: format, limits: limits.WithDefaults()}
}

// Name returns the source's identifier.
func (s *FileSource) Name() string { return s.name }

// CacheKey fingerprints the source by format and path.
func (s *FileSource) CacheKey() string { return s.format.String() + ":" + s.path }

// Stream reads the file lazily, surfacing any read or parse error on the first pull rather than at construction time.
func (s *FileSource) Stream(context.Context) *stream.Pipeline[Record] {
	return stream.FromFunc(func(ctx context.Context) stream.Iterator[Record] {
		p, err := s.read()
		if err != nil {
			return &failIter{err: err}
		}
		return p.Iter(ctx)
	})
}

// read dispatches to the reader for the configured format.
func (s *FileSource) read() (*stream.Pipeline[Record], error) {
	switch s.format {
	case FormatCSV:
		return ReadCSV(s.path, s.limits)
	case FormatJSONArray:
		return ReadJSONArray(s.path, s.limits)
	case FormatJSONLines:
		return ReadJSONLines(s.path, s.limits)
	default:
		return nil, apperrors.InvalidInput("format", "unknown record format")
	}
}

// FileTarget is a [stage.Target] that writes records to a file in one of the supported [Format]s via an atomic replace, so a partial write never leaves a truncated file. It accumulates records across publishes so a multi-source run writes every source's records to the single file rather than clobbering it.
type FileTarget struct {
	name    string
	path    string
	format  Format
	records []Record
}

// NewFileTarget returns a file-backed record target in the given format.
func NewFileTarget(name, path string, format Format) *FileTarget {
	return &FileTarget{name: name, path: path, format: format}
}

// Name returns the target's identifier.
func (t *FileTarget) Name() string { return t.name }

// Publish appends the records to the target's accumulated set and rewrites the file, reporting the total record count now on disk. Publishing is driven from the collector's single main loop, so the accumulator is not shared across goroutines.
func (t *FileTarget) Publish(ctx context.Context, items *stream.Pipeline[Record]) (stage.PublishResult, error) {
	write, err := t.writer()
	if err != nil {
		return stage.PublishResult{}, err
	}
	collected, err := stream.Collect(ctx, items)
	if err != nil {
		return stage.PublishResult{}, err
	}
	t.records = append(t.records, collected...)

	var buf bytes.Buffer
	n, err := write(ctx, &buf, stream.FromSlice(t.records))
	if err != nil {
		return stage.PublishResult{}, err
	}
	if err := fs.WriteAtomicReplace(t.path, buf.Bytes(), "record-"); err != nil {
		return stage.PublishResult{}, err
	}
	return stage.PublishResult{
		TargetName:       t.name,
		Location:         t.path,
		RecordsPublished: n,
	}, nil
}

// writer returns the writer for the configured format.
func (t *FileTarget) writer() (func(context.Context, io.Writer, *stream.Pipeline[Record]) (int, error), error) {
	switch t.format {
	case FormatCSV:
		return WriteCSV, nil
	case FormatJSONArray:
		return WriteJSONArray, nil
	case FormatJSONLines:
		return WriteJSONLines, nil
	default:
		return nil, apperrors.InvalidInput("format", "unknown record format")
	}
}

// failIter yields err on first pull, deferring a read error into the stream.
type failIter struct{ err error }

func (it *failIter) Next(context.Context) (Record, bool, error) { return Record{}, false, it.err }
func (it *failIter) Close() error                               { return nil }
