package bench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
	gofs "github.com/kbukum/gokit/fs"
	"github.com/kbukum/gokit/util"
)

// ErrRunIDSeparator means a run ID contains a path separator and cannot be safely mapped to one file.
var ErrRunIDSeparator = errors.New("run ID must not contain path separators")

// RunStorage persists benchmark results.
type RunStorage interface {
	Save(ctx context.Context, result *RunResult) (string, error)
	Load(ctx context.Context, runID string) (*RunResult, error)
	Latest(ctx context.Context) (*RunResult, error)
	List(ctx context.Context, opts ...ListOption) ([]RunSummary, error)
}

// ListOption configures result listing.
type ListOption func(*listConfig)

type listConfig struct {
	limit   int
	tag     string
	dataset string
}

// WithLimit sets the maximum number of results to return.
func WithLimit(n int) ListOption {
	return func(c *listConfig) { c.limit = n }
}

// WithTagFilter filters results by tag.
func WithTagFilter(tag string) ListOption {
	return func(c *listConfig) { c.tag = tag }
}

// WithDatasetFilter filters results by dataset name.
func WithDatasetFilter(dataset string) ListOption {
	return func(c *listConfig) { c.dataset = dataset }
}

// ListParams holds the resolved parameters from ListOption values.
type ListParams struct {
	Limit   int
	Tag     string
	Dataset string
}

// ResolveListOptions applies the given options and returns the resolved parameters.
// This is useful for external RunStorage implementations that need to inspect filter values.
func ResolveListOptions(opts ...ListOption) ListParams {
	cfg := listConfig{limit: 100}
	for _, o := range opts {
		o(&cfg)
	}
	return ListParams{
		Limit:   cfg.limit,
		Tag:     cfg.tag,
		Dataset: cfg.dataset,
	}
}

// FileStorage stores results as JSON files on disk.
type FileStorage struct {
	dir string
}

// NewFileStorage creates a FileStorage that persists results under dir.
func NewFileStorage(dir string) *FileStorage {
	return &FileStorage{dir: dir}
}

// Save writes the RunResult as a JSON file named {runID}.json.
func (fs *FileStorage) Save(_ context.Context, result *RunResult) (string, error) {
	if err := os.MkdirAll(fs.dir, 0o750); err != nil {
		return "", fmt.Errorf("bench: create storage dir: %w", err)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("bench: marshal result: %w", err)
	}
	path, err := fs.runPath(result.ID)
	if err != nil {
		return "", err
	}
	if err := util.WriteFile(path, data); err != nil {
		return "", fmt.Errorf("bench: write result file: %w", err)
	}
	return result.ID, nil
}

// Load reads a RunResult from disk by run ID.
func (fs *FileStorage) Load(_ context.Context, runID string) (*RunResult, error) {
	path, err := fs.runPath(runID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("bench: read result %s: %w", runID, err)
	}
	var result RunResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("bench: parse result %s: %w", runID, err)
	}
	return &result, nil
}

func (fs *FileStorage) runPath(runID string) (string, error) {
	if err := validateRunID(runID); err != nil {
		return "", err
	}
	path, err := gofs.SafeJoin(fs.dir, runID+".json")
	if err != nil {
		return "", invalidRunIDError(runID, err)
	}
	return path, nil
}

func validateRunID(runID string) error {
	if runID == "" {
		return invalidRunIDError(runID, gofs.ErrPathEmpty)
	}
	// Validate for traversal first so a "../x" ID reports the parent-dir cause,
	// then independently reject separators — harmless IDs like "release..candidate"
	// (no "../" segment) stay loadable.
	if err := gofs.ValidateRelativePath(runID); err != nil {
		return invalidRunIDError(runID, err)
	}
	if strings.ContainsAny(runID, `/\`) {
		return invalidRunIDError(runID, ErrRunIDSeparator)
	}
	return nil
}

func invalidRunIDError(runID string, cause error) error {
	return apperrors.New(apperrors.ErrCodeInvalidInput,
		fmt.Sprintf("invalid benchmark run ID %q", runID), http.StatusBadRequest).WithCause(cause)
}

// Latest returns the most recent RunResult by timestamp.
func (fs *FileStorage) Latest(ctx context.Context) (*RunResult, error) {
	summaries, err := fs.List(ctx, WithLimit(1))
	if err != nil {
		return nil, err
	}
	if len(summaries) == 0 {
		return nil, fmt.Errorf("bench: no results found")
	}
	return fs.Load(ctx, summaries[0].ID)
}

// List returns summaries of stored results, sorted by timestamp descending.
func (fs *FileStorage) List(_ context.Context, opts ...ListOption) ([]RunSummary, error) {
	cfg := listConfig{limit: 100}
	for _, o := range opts {
		o(&cfg)
	}

	entries, err := os.ReadDir(fs.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("bench: read storage dir: %w", err)
	}

	var summaries []RunSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(fs.dir, entry.Name()))
		if err != nil {
			continue
		}
		var result RunResult
		if err := json.Unmarshal(data, &result); err != nil {
			continue
		}

		if cfg.tag != "" && result.Tag != cfg.tag {
			continue
		}
		if cfg.dataset != "" && result.Dataset.Name != cfg.dataset {
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

		summaries = append(summaries, RunSummary{
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

	if cfg.limit > 0 && len(summaries) > cfg.limit {
		summaries = summaries[:cfg.limit]
	}

	return summaries, nil
}
