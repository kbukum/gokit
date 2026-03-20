package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kbukum/gokit/pipeline"
)

// DatasetManifest describes a labeled dataset on disk.
type DatasetManifest struct {
	Name    string           `json:"name"`
	Version string           `json:"version"`
	Samples []ManifestSample `json:"samples"`
}

// ManifestSample is one entry in a dataset manifest file.
type ManifestSample struct {
	ID     string         `json:"id"`
	File   string         `json:"file"`
	Label  string         `json:"label"`
	Source string         `json:"source,omitempty"`
	Meta   map[string]any `json:"metadata,omitempty"`
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

// loadManifest reads and parses the manifest file.
func (d *DatasetLoader[L]) loadManifest() (*DatasetManifest, error) {
	path := filepath.Join(d.dir, d.cfg.manifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("bench: read manifest %s: %w", path, err)
	}
	var m DatasetManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("bench: parse manifest %s: %w", path, err)
	}
	return &m, nil
}

// Manifest returns the parsed manifest.
func (d *DatasetLoader[L]) Manifest() (*DatasetManifest, error) {
	return d.loadManifest()
}

// All loads all samples into memory.
func (d *DatasetLoader[L]) All(ctx context.Context) ([]Sample[L], error) {
	return pipeline.Collect(ctx, d.Pipeline())
}

// Pipeline returns a Pipeline[Sample[L]] for composition.
func (d *DatasetLoader[L]) Pipeline() *pipeline.Pipeline[Sample[L]] {
	return pipeline.FromFunc(func(ctx context.Context) pipeline.Iterator[Sample[L]] {
		iter, err := d.Iterator(ctx)
		if err != nil {
			return &errIter[Sample[L]]{err: err}
		}
		return iter
	})
}

// Iterator returns a pipeline.Iterator that lazily loads samples.
func (d *DatasetLoader[L]) Iterator(ctx context.Context) (pipeline.Iterator[Sample[L]], error) {
	manifest, err := d.loadManifest()
	if err != nil {
		return nil, err
	}
	samples := manifest.Samples
	if d.cfg.filter != nil {
		var filtered []ManifestSample
		for _, s := range samples {
			if d.cfg.filter(s) {
				filtered = append(filtered, s)
			}
		}
		samples = filtered
	}
	return &datasetIter[L]{
		dir:     d.dir,
		mapper:  d.mapper,
		samples: samples,
	}, nil
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
	dir     string
	mapper  LabelMapper[L]
	samples []ManifestSample
	index   int
}

func (it *datasetIter[L]) Next(_ context.Context) (Sample[L], bool, error) {
	if it.index >= len(it.samples) {
		var zero Sample[L]
		return zero, false, nil
	}
	ms := it.samples[it.index]
	it.index++

	label, err := it.mapper(ms.Label)
	if err != nil {
		var zero Sample[L]
		return zero, false, fmt.Errorf("bench: map label %q for sample %s: %w", ms.Label, ms.ID, err)
	}

	var input []byte
	if ms.File != "" {
		input, err = os.ReadFile(filepath.Join(it.dir, ms.File))
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

func (it *datasetIter[L]) Close() error { return nil }

// errIter always returns an error on the first Next call.
type errIter[T any] struct {
	err  error
	done bool
}

func (it *errIter[T]) Next(_ context.Context) (T, bool, error) {
	var zero T
	if it.done {
		return zero, false, nil
	}
	it.done = true
	return zero, false, it.err
}

func (it *errIter[T]) Close() error { return nil }
