package bench

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
	gofs "github.com/kbukum/gokit/fs"
)

// ErrRunIDSeparator rejects IDs that cannot map to a single summary key.
var ErrRunIDSeparator = errors.New("run ID must not contain path separators")

// RunStorage is the read-only result contract used by CLIRunner.
type RunStorage interface {
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

// WithLimit sets the maximum number of listed results.
func WithLimit(n int) ListOption { return func(c *listConfig) { c.limit = n } }

// WithTagFilter filters listed results by tag.
func WithTagFilter(tag string) ListOption { return func(c *listConfig) { c.tag = tag } }

// WithDatasetFilter filters listed results by dataset name.
func WithDatasetFilter(dataset string) ListOption { return func(c *listConfig) { c.dataset = dataset } }

// ListParams holds resolved listing options for result-store adapters.
type ListParams struct {
	Limit   int
	Tag     string
	Dataset string
}

// ResolveListOptions applies filters with a default limit of 100.
func ResolveListOptions(opts ...ListOption) ListParams {
	cfg := listConfig{limit: 100}
	for _, opt := range opts {
		opt(&cfg)
	}
	return ListParams{Limit: cfg.limit, Tag: cfg.tag, Dataset: cfg.dataset}
}

func validateRunID(runID string) error {
	if runID == "" {
		return invalidRunIDError(runID, gofs.ErrPathEmpty)
	}
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
		fmt.Sprintf("invalid benchmark run ID %q", runID)).WithCause(cause)
}
