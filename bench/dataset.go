package bench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	gofs "github.com/kbukum/gokit/fs"
	"github.com/kbukum/gokit/stream"
)

// Dataset describes and opens a fresh, streaming sample traversal.
type Dataset[L comparable] interface {
	Describe(context.Context) (DatasetDescriptor, error)
	Iterator(context.Context) (stream.Iterator[Sample[L]], error)
}

// DatasetDescriptor pins the dataset's declared name and version.
type DatasetDescriptor struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type generatorDataset[L comparable] struct {
	descriptor DatasetDescriptor
	open       func(context.Context) (stream.Iterator[Sample[L]], error)
}

// NewGeneratorDataset injects a fresh iterator factory, without retaining samples.
func NewGeneratorDataset[L comparable](descriptor DatasetDescriptor, open func(context.Context) (stream.Iterator[Sample[L]], error)) Dataset[L] {
	return &generatorDataset[L]{descriptor: descriptor, open: open}
}

func (d *generatorDataset[L]) Describe(ctx context.Context) (DatasetDescriptor, error) {
	return d.descriptor, ctx.Err()
}

func (d *generatorDataset[L]) Iterator(ctx context.Context) (stream.Iterator[Sample[L]], error) {
	return d.open(ctx)
}

// NewSliceDataset uses caller-owned samples for small in-memory datasets.
func NewSliceDataset[L comparable](descriptor DatasetDescriptor, samples []Sample[L]) Dataset[L] {
	return NewGeneratorDataset(descriptor, func(ctx context.Context) (stream.Iterator[Sample[L]], error) {
		return &sliceDatasetIterator[L]{samples: samples}, ctx.Err()
	})
}

type sliceDatasetIterator[L comparable] struct {
	samples []Sample[L]
	index   int
}

func (it *sliceDatasetIterator[L]) Next(ctx context.Context) (Sample[L], bool, error) {
	if err := ctx.Err(); err != nil {
		return Sample[L]{}, false, err
	}
	if it.index == len(it.samples) {
		return Sample[L]{}, false, nil
	}
	s := it.samples[it.index]
	it.index++
	return s, true, nil
}
func (it *sliceDatasetIterator[L]) Close() error { it.samples = nil; return nil }

// DatasetManifest describes a labeled dataset on disk.
type DatasetManifest struct {
	Name    string           `json:"name"`
	Version string           `json:"version"`
	Samples []ManifestSample `json:"samples"`
}

// ManifestSample is one entry in a dataset manifest file.
type ManifestSample struct {
	ID     string                     `json:"id"`
	File   string                     `json:"file"`
	Label  string                     `json:"label"`
	Source string                     `json:"source,omitempty"`
	Meta   map[string]json.RawMessage `json:"metadata,omitempty"`
}

// DatasetOption configures dataset loading.
type DatasetOption func(*datasetConfig)

type datasetConfig struct {
	manifestFile string
	filter       func(ManifestSample) bool
}

// WithManifestFile sets the manifest filename (default: "manifest.json").
func WithManifestFile(name string) DatasetOption {
	return func(c *datasetConfig) { c.manifestFile = name }
}

// DatasetLoader loads labeled samples from a manifest file.
type DatasetLoader[L comparable] struct {
	dir    string
	mapper LabelMapper[L]
	cfg    datasetConfig
}

// NewDatasetLoader creates a loader for the given directory.
func NewDatasetLoader[L comparable](dir string, mapper LabelMapper[L], opts ...DatasetOption) *DatasetLoader[L] {
	cfg := datasetConfig{manifestFile: "manifest.json"}
	for _, o := range opts {
		o(&cfg)
	}
	return &DatasetLoader[L]{dir: dir, mapper: mapper, cfg: cfg}
}

// Describe scans metadata without retaining the manifest's sample array.
func (d *DatasetLoader[L]) Describe(ctx context.Context) (descriptor DatasetDescriptor, err error) {
	f, err := d.openManifest()
	if err != nil {
		return descriptor, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	dec := json.NewDecoder(io.LimitReader(f, DefaultLimits().MaxRunBytes))
	token, err := dec.Token()
	if err != nil {
		return descriptor, err
	}
	if token != json.Delim('{') {
		return descriptor, fmt.Errorf("bench: manifest must be an object")
	}
	for dec.More() {
		if err := ctx.Err(); err != nil {
			return descriptor, err
		}
		key, err := dec.Token()
		if err != nil {
			return descriptor, err
		}
		switch key {
		case "name":
			err = dec.Decode(&descriptor.Name)
		case "version":
			err = dec.Decode(&descriptor.Version)
		default:
			err = skipJSON(ctx, dec)
		}
		if err != nil {
			return descriptor, err
		}
	}
	return descriptor, finishObject(dec)
}

func (d *DatasetLoader[L]) openManifest() (*os.File, error) {
	if err := gofs.ValidateRelativePath(d.cfg.manifestFile); err != nil {
		return nil, err
	}
	path, err := gofs.ConfineExistingPath(d.dir, d.cfg.manifestFile)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func finishObject(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('}') {
		return fmt.Errorf("bench: expected manifest object end")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return fmt.Errorf("bench: trailing manifest data: %w", err)
		}
		return fmt.Errorf("bench: trailing manifest data")
	}
	return nil
}

func skipJSON(ctx context.Context, dec *json.Decoder) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := token.(json.Delim); ok && (delim == '[' || delim == '{') {
		for dec.More() {
			if skipErr := skipJSON(ctx, dec); skipErr != nil {
				return skipErr
			}
		}
		_, err = dec.Token()
	}
	return err
}

// Pipeline returns a Pipeline[Sample[L]] for composition.
func (d *DatasetLoader[L]) Pipeline() *stream.Pipeline[Sample[L]] {
	return stream.FromFunc(func(ctx context.Context) stream.Iterator[Sample[L]] {
		iter, err := d.Iterator(ctx)
		if err != nil {
			return &errIter[Sample[L]]{err: err}
		}
		return iter
	})
}

// Iterator returns a stream.Iterator that lazily loads samples.
func (d *DatasetLoader[L]) Iterator(ctx context.Context) (stream.Iterator[Sample[L]], error) {
	f, err := d.openManifest()
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(io.LimitReader(f, DefaultLimits().MaxRunBytes))
	err = findSamples(ctx, dec)
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return &datasetIter[L]{dir: d.dir, mapper: d.mapper, dec: dec, file: f, filter: d.cfg.filter}, nil
}

func findSamples(ctx context.Context, dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return fmt.Errorf("bench: manifest must be an object")
	}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return err
		}
		if key == "samples" {
			token, err := dec.Token()
			if err != nil {
				return err
			}
			if token != json.Delim('[') {
				return fmt.Errorf("bench: samples must be an array")
			}
			return nil
		}
		if err := skipJSON(ctx, dec); err != nil {
			return err
		}
	}
	return fmt.Errorf("bench: manifest has no samples")
}

// Filter returns a new loader that only yields matching samples.
func (d *DatasetLoader[L]) Filter(fn func(ManifestSample) bool) *DatasetLoader[L] {
	return &DatasetLoader[L]{
		dir:    d.dir,
		mapper: d.mapper,
		cfg: datasetConfig{
			manifestFile: d.cfg.manifestFile,
			filter:       fn,
		},
	}
}

// datasetIter iterates over manifest samples.
type datasetIter[L comparable] struct {
	dir    string
	mapper LabelMapper[L]
	dec    *json.Decoder
	file   *os.File
	filter func(ManifestSample) bool
	done   bool
}

func (it *datasetIter[L]) Next(ctx context.Context) (Sample[L], bool, error) {
	if it.done {
		return Sample[L]{}, false, nil
	}
	var ms ManifestSample
	for {
		if err := ctx.Err(); err != nil {
			return Sample[L]{}, false, err
		}
		if !it.dec.More() {
			it.done = true
			return Sample[L]{}, false, it.finish(ctx)
		}
		ms = ManifestSample{}
		if err := it.dec.Decode(&ms); err != nil {
			return Sample[L]{}, false, err
		}
		if it.filter == nil || it.filter(ms) {
			break
		}
	}

	label, err := it.mapper(ms.Label)
	if err != nil {
		var zero Sample[L]
		return zero, false, fmt.Errorf("bench: map label %q for sample %s: %w", ms.Label, ms.ID, err)
	}

	var input []byte
	if ms.File != "" {
		var path string
		if pathErr := gofs.ValidateRelativePath(ms.File); pathErr != nil {
			return Sample[L]{}, false, pathErr
		}
		path, err = gofs.ConfineExistingPath(it.dir, ms.File)
		if err != nil {
			return Sample[L]{}, false, err
		}
		var f *os.File
		f, err = os.Open(path)
		if err != nil {
			return Sample[L]{}, false, err
		}
		input, err = io.ReadAll(io.LimitReader(f, (1<<20)+1))
		err = errors.Join(err, f.Close())
		if len(input) > 1<<20 {
			return Sample[L]{}, false, fmt.Errorf("bench: input exceeds 1 MiB")
		}
		if err != nil {
			var zero Sample[L]
			return zero, false, fmt.Errorf("bench: read sample file %s: %w", ms.File, err)
		}
	}

	return Sample[L]{
		ID:       ms.ID,
		Input:    input,
		Label:    label,
		Source:   ms.Source,
		Metadata: ms.Meta,
	}, true, nil
}

func (it *datasetIter[L]) finish(ctx context.Context) error {
	token, err := it.dec.Token()
	if err != nil {
		return err
	}
	if token != json.Delim(']') {
		return fmt.Errorf("bench: expected samples array end")
	}
	for it.dec.More() {
		key, err := it.dec.Token()
		if err != nil {
			return err
		}
		if key == "samples" {
			return fmt.Errorf("bench: duplicate samples array")
		}
		if err := skipJSON(ctx, it.dec); err != nil {
			return err
		}
	}
	return finishObject(it.dec)
}

func (it *datasetIter[L]) Close() error {
	it.done = true
	if it.file == nil {
		return nil
	}
	err := it.file.Close()
	it.file = nil
	return err
}

// errIter always returns an error on the first Next call.
type errIter[T any] struct {
	err  error
	done bool
}

func (it *errIter[T]) Next(_ context.Context) (zero T, _ bool, _ error) {
	if it.done {
		return zero, false, nil
	}
	it.done = true
	return zero, false, it.err
}

func (it *errIter[T]) Close() error { return nil }
