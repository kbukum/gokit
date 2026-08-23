package vectorstore

import (
	"errors"
	"testing"
)

func TestConfigAppliesDefaultLimits(t *testing.T) {
	t.Parallel()

	cfg := Config{}
	cfg.ApplyDefaults()

	want := DefaultLimits()
	if cfg.Limits != want {
		t.Fatalf("Config.Limits = %+v, want %+v", cfg.Limits, want)
	}
}

func TestConfigKeepsExplicitLimits(t *testing.T) {
	t.Parallel()

	cfg := Config{Limits: VectorStoreLimits{MaxSearchLimit: 5}}
	cfg.ApplyDefaults()

	if cfg.Limits.MaxSearchLimit != 5 {
		t.Fatalf("MaxSearchLimit = %d, want 5 (explicit value overwritten)", cfg.Limits.MaxSearchLimit)
	}
	if cfg.Limits.MaxVectorDimensions != DefaultMaxVectorDimensions {
		t.Fatalf("MaxVectorDimensions = %d, want default %d", cfg.Limits.MaxVectorDimensions, DefaultMaxVectorDimensions)
	}
}

func TestLimitsValidateDimensions(t *testing.T) {
	t.Parallel()

	l := DefaultLimits()
	for _, d := range []int{0, -1, DefaultMaxVectorDimensions + 1} {
		if err := l.ValidateDimensions(d); err == nil {
			t.Fatalf("ValidateDimensions(%d) = nil, want error", d)
		}
	}
	if err := l.ValidateDimensions(128); err != nil {
		t.Fatalf("ValidateDimensions(128) = %v, want nil", err)
	}
}

func TestLimitsValidateSearchLimit(t *testing.T) {
	t.Parallel()

	l := DefaultLimits()
	if err := l.ValidateSearchLimit(-1); err == nil {
		t.Fatal("ValidateSearchLimit(-1) = nil, want error")
	}
	if err := l.ValidateSearchLimit(DefaultMaxSearchLimit + 1); err == nil {
		t.Fatal("ValidateSearchLimit(over max) = nil, want error")
	}
	if err := l.ValidateSearchLimit(0); err != nil {
		t.Fatalf("ValidateSearchLimit(0) = %v, want nil (empty result is allowed)", err)
	}
}

func TestPayloadValidateFieldCount(t *testing.T) {
	t.Parallel()

	l := VectorStoreLimits{MaxPayloadFields: 2, MaxPayloadBytes: DefaultMaxPayloadBytes}
	p := NewPointPayload().WithField("a", 1).WithField("b", 2).WithField("c", 3)
	var limitErr *LimitExceededError
	if err := p.Validate(l); !errors.As(err, &limitErr) {
		t.Fatalf("Validate = %v, want LimitExceededError for field count", err)
	}
}

func TestPayloadValidateByteSize(t *testing.T) {
	t.Parallel()

	l := VectorStoreLimits{MaxPayloadFields: DefaultMaxPayloadFields, MaxPayloadBytes: 8}
	p := NewPointPayload().WithField("field", "a-long-value-well-over-eight-bytes")
	if err := p.Validate(l); err == nil {
		t.Fatal("Validate = nil, want error for oversized payload bytes")
	}
}

func TestPayloadValidateAllowsWithinLimits(t *testing.T) {
	t.Parallel()

	l := DefaultLimits()
	p := NewPointPayload().WithField("kind", "doc").WithField("n", 3)
	if err := p.Validate(l); err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
	var nilPayload *PointPayload
	if err := nilPayload.Validate(l); err != nil {
		t.Fatalf("nil payload Validate = %v, want nil", err)
	}
}

func TestFilterValidateConditionCount(t *testing.T) {
	t.Parallel()

	l := DefaultLimits()
	l.MaxFilterConditions = 1
	f := NewSearchFilter().MustMatch("a", 1).MustMatch("b", 2)
	if err := f.Validate(l); err == nil {
		t.Fatal("Validate = nil, want error for too many filter conditions")
	}
	var nilFilter *SearchFilter
	if err := nilFilter.Validate(l); err != nil {
		t.Fatalf("nil filter Validate = %v, want nil", err)
	}
}

func TestInMemoryStoreEnsureCollectionRejectsOversizeDimensions(t *testing.T) {
	t.Parallel()

	store, err := NewInMemoryStoreWithConfig(Config{Limits: VectorStoreLimits{MaxVectorDimensions: 4}})
	if err != nil {
		t.Fatalf("NewInMemoryStoreWithConfig: %v", err)
	}
	if err := store.EnsureCollection(t.Context(), "c", 8); err == nil {
		t.Fatal("EnsureCollection accepted dimensions above MaxVectorDimensions")
	}
}

func TestInMemoryStoreSearchRejectsOverMaxLimit(t *testing.T) {
	t.Parallel()

	store, err := NewInMemoryStoreWithConfig(Config{Limits: VectorStoreLimits{MaxSearchLimit: 2}})
	if err != nil {
		t.Fatalf("NewInMemoryStoreWithConfig: %v", err)
	}
	ctx := t.Context()
	if err := store.EnsureCollection(ctx, "c", 2); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}
	_, err = store.Search(ctx, "c", SearchQuery{Vector: []float32{1, 0}, Limit: 5})
	if err == nil {
		t.Fatal("Search accepted a limit above MaxSearchLimit")
	}
}

func TestInMemoryStoreUpsertRejectsOversizePayload(t *testing.T) {
	t.Parallel()

	store, err := NewInMemoryStoreWithConfig(Config{Limits: VectorStoreLimits{MaxPayloadFields: 1}})
	if err != nil {
		t.Fatalf("NewInMemoryStoreWithConfig: %v", err)
	}
	ctx := t.Context()
	if err := store.EnsureCollection(ctx, "c", 2); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}
	payload := NewPointPayload().WithField("a", 1).WithField("b", 2)
	err = store.Upsert(ctx, "c", Point{ID: "1", Vector: []float32{1, 0}, Payload: payload})
	if err == nil {
		t.Fatal("Upsert accepted a payload exceeding MaxPayloadFields")
	}
}
