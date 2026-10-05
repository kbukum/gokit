package bench

import (
	"context"
	"encoding/json"
	"testing"
)

func TestResultStoreReadBudgets(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*RunResult)
	}{
		{"negative segment count", func(r *RunResult) { r.RecordSegments[0] = -1 }},
		{"inflated read limit", func(r *RunResult) { r.RecordReadLimit = 1 << 30 }},
		{"zero read limit", func(r *RunResult) { r.RecordReadLimit = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			objects := NewDirStore(t.TempDir())
			store := NewResultStore(objects)
			result := &RunResult{ID: "untrusted", RecordSegments: []int{1}, RecordReadLimit: 1024}
			tc.change(result)
			raw, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if err := objects.Put(t.Context(), summaryKey(result.ID), raw); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Records(t.Context(), result.ID, 0); err == nil {
				t.Fatal("untrusted record manifest accepted")
			}
		})
	}
}

func TestResultStoreConfiguredLimits(t *testing.T) {
	t.Parallel()
	store := NewResultStore(NewDirStore(t.TempDir()), WithSummaryReadLimit(8192), WithRecordReadLimit(32768))
	limits := DefaultLimits()
	if _, err := store.begin(t.Context(), "mismatch", 1, limits); err == nil {
		t.Fatal("writer may exceed reader budgets")
	}
	limits.MaxSummaryBytes, limits.MaxRecordBytes, limits.SegmentBytes = 8192, 32768, 8192
	w, err := store.begin(t.Context(), "bounded", 1, limits)
	if err != nil {
		t.Fatal(err)
	}
	defer w.release()
	if err := w.append(t.Context(), 0, Record{ID: "one", Label: json.RawMessage(`"yes"`), Predicted: json.RawMessage(`"yes"`)}); err != nil {
		t.Fatal(err)
	}
	if err := w.commit(t.Context(), &RunResult{ID: "bounded"}); err != nil {
		t.Fatal(err)
	}
	it, err := store.Records(t.Context(), "bounded", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := it.Next(t.Context()); !ok || err != nil {
		t.Fatalf("configured reload: %v %v", ok, err)
	}
	if err := it.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestResultStoreRejectsCorruptSummary(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{`, `{"version":"1.0","id":"bad"}`, `{"version":"2.0","id":"other"}`} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			objects := NewDirStore(t.TempDir())
			if err := objects.Put(t.Context(), summaryKey("bad"), []byte(raw)); err != nil {
				t.Fatal(err)
			}
			if _, err := NewResultStore(objects).List(t.Context()); err == nil {
				t.Fatal("corrupt summary silently skipped")
			}
		})
	}
}

type fixtureStore struct{ *ResultStore }

func newFixtureStore(dir string) *fixtureStore {
	return &fixtureStore{NewResultStore(NewDirStore(dir))}
}

func (s *fixtureStore) Save(ctx context.Context, result *RunResult) (string, error) {
	w, err := s.begin(ctx, result.ID, len(result.Branches), DefaultLimits())
	if err != nil {
		return "", err
	}
	defer w.release()
	if err := w.commit(ctx, result); err != nil {
		return "", err
	}
	return result.ID, nil
}

func sampleResults(t *testing.T, store *ResultStore, result *RunResult) []SampleResult {
	t.Helper()
	iter, err := store.Records(context.Background(), result.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := iter.Close(); err != nil {
			t.Error(err)
		}
	}()
	var samples []SampleResult
	for {
		r, ok, err := iter.Next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		samples = append(samples, SampleResult{ID: r.ID, Label: r.LabelText(), Predicted: r.PredictedText(), Correct: r.Correct, Duration: r.Latency, Score: r.Score, Error: r.Error})
	}
	return samples
}
