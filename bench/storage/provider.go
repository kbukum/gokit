package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kbukum/gokit/bench"
	gostorage "github.com/kbukum/gokit/storage"
)

// Option configures a ProviderStorage.
type Option func(*ProviderStorage)

// WithPrefix sets the key prefix for stored results. Default is "bench/".
func WithPrefix(prefix string) Option {
	return func(s *ProviderStorage) { s.prefix = prefix }
}

// ProviderStorage implements bench.RunStorage using a gokit/storage.Storage backend.
type ProviderStorage struct {
	store  gostorage.Storage
	prefix string
}

// NewProviderStorage creates a new ProviderStorage wrapping the given storage backend.
func NewProviderStorage(store gostorage.Storage, opts ...Option) *ProviderStorage {
	s := &ProviderStorage{
		store:  store,
		prefix: "bench/",
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// key builds the full storage path for a run ID.
func (s *ProviderStorage) key(runID string) string {
	return s.prefix + runID + ".json"
}

// Save persists a RunResult to the storage backend.
func (s *ProviderStorage) Save(ctx context.Context, result *bench.RunResult) (string, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("bench/storage: marshal result: %w", err)
	}
	if err := s.store.Upload(ctx, s.key(result.ID), bytes.NewReader(data)); err != nil {
		return "", fmt.Errorf("bench/storage: upload result: %w", err)
	}
	return result.ID, nil
}

// Load retrieves a RunResult by run ID.
func (s *ProviderStorage) Load(ctx context.Context, runID string) (*bench.RunResult, error) {
	rc, err := s.store.Download(ctx, s.key(runID))
	if err != nil {
		return nil, fmt.Errorf("bench/storage: download result %s: %w", runID, err)
	}
	defer func() { _ = rc.Close() }()

	var result bench.RunResult
	if err := json.NewDecoder(rc).Decode(&result); err != nil {
		return nil, fmt.Errorf("bench/storage: decode result %s: %w", runID, err)
	}
	return &result, nil
}

// Latest returns the most recent RunResult by listing and sorting stored results.
func (s *ProviderStorage) Latest(ctx context.Context) (*bench.RunResult, error) {
	summaries, err := s.List(ctx, bench.WithLimit(1))
	if err != nil {
		return nil, err
	}
	if len(summaries) == 0 {
		return nil, fmt.Errorf("bench/storage: no results found")
	}
	return s.Load(ctx, summaries[0].ID)
}

// List returns summaries of stored results, sorted by timestamp descending.
func (s *ProviderStorage) List(ctx context.Context, opts ...bench.ListOption) ([]bench.RunSummary, error) {
	params := bench.ResolveListOptions(opts...)

	files, err := s.store.List(ctx, s.prefix)
	if err != nil {
		return nil, fmt.Errorf("bench/storage: list results: %w", err)
	}

	var summaries []bench.RunSummary
	for _, f := range files {
		if !strings.HasSuffix(f.Path, ".json") {
			continue
		}

		rc, err := s.store.Download(ctx, f.Path)
		if err != nil {
			continue
		}

		var result bench.RunResult
		decErr := json.NewDecoder(rc).Decode(&result)
		_ = rc.Close()
		if decErr != nil {
			continue
		}

		if params.Tag != "" && result.Tag != params.Tag {
			continue
		}
		if params.Dataset != "" && result.Dataset.Name != params.Dataset {
			continue
		}

		var f1 float64
		for _, m := range result.Metrics {
			if v, ok := m.Values["f1"]; ok {
				f1 = v
				break
			}
			if m.Name == "classification" || m.Name == "multi_class_classification" {
				f1 = m.Value
				break
			}
		}

		summaries = append(summaries, bench.RunSummary{
			ID:        result.ID,
			Timestamp: result.Timestamp,
			Tag:       result.Tag,
			Dataset:   result.Dataset.Name,
			F1:        f1,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Timestamp.After(summaries[j].Timestamp)
	})

	if params.Limit > 0 && len(summaries) > params.Limit {
		summaries = summaries[:params.Limit]
	}

	return summaries, nil
}

// Compile-time assertion that ProviderStorage implements bench.RunStorage.
var _ bench.RunStorage = (*ProviderStorage)(nil)
