package metric

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/kbukum/gokit/bench"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/llm"
)

func tokenScored(prediction, reference string) bench.ScoredSample[string] {
	return bench.ScoredSample[string]{
		Sample:     bench.Sample[string]{ID: "s", Label: reference},
		Prediction: bench.Prediction[string]{Label: prediction},
	}
}

func newTokenStats(t *testing.T, counter llm.TokenCounter) Metric[string] {
	t.Helper()
	m, err := TokenStats[string](counter)
	if err != nil {
		t.Fatalf("TokenStats: %v", err)
	}
	return m
}

func TestTokenStatsNameEmbedsCounter(t *testing.T) {
	t.Parallel()

	m := newTokenStats(t, llm.HeuristicTokenCounter{})
	if got, want := m.Name(), "token_stats[heuristic]"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestTokenStatsRejectsNilCounter(t *testing.T) {
	t.Parallel()

	assertInvalidInput := func(t *testing.T, err error) {
		t.Helper()
		appErr, ok := apperrors.AsAppError(err)
		if !ok {
			t.Fatalf("error is not an *apperrors.AppError: %v", err)
		}
		if appErr.Code != apperrors.ErrCodeInvalidInput {
			t.Errorf("Code = %q, want %q", appErr.Code, apperrors.ErrCodeInvalidInput)
		}
		if appErr.HTTPStatus != http.StatusBadRequest {
			t.Errorf("HTTPStatus = %d, want %d", appErr.HTTPStatus, http.StatusBadRequest)
		}
	}

	_, err := TokenStats[string](nil)
	if err == nil {
		t.Fatal("expected error for nil counter, got nil")
	}
	assertInvalidInput(t, err)

	// A typed-nil interface value must also be rejected, not stored and later
	// dispatched through (which would panic in Compute).
	var typedNil *failingCounter
	_, err = TokenStats[string](typedNil)
	if err == nil {
		t.Fatal("expected error for typed-nil counter, got nil")
	}
	assertInvalidInput(t, err)
}

func TestTokenStatsEmptyInputIsZeroed(t *testing.T) {
	t.Parallel()

	res := newTokenStats(t, llm.HeuristicTokenCounter{}).Compute(nil)
	if res.Value != 0 {
		t.Errorf("Value = %f, want 0", res.Value)
	}
	for _, key := range []string{
		"predicted_tokens_total", "predicted_tokens_avg",
		"reference_tokens_total", "reference_tokens_avg",
	} {
		if v, ok := res.Values[key]; !ok || v != 0 {
			t.Errorf("Values[%q] = %v (present=%v), want 0", key, v, ok)
		}
	}
}

func TestTokenStatsCountsPredictedAndReference(t *testing.T) {
	t.Parallel()

	// Heuristic: (len+3)/4. "abcdefgh"=2, "abcd"=1 predicted; refs "xy"=1, "z"=1.
	scored := []bench.ScoredSample[string]{
		tokenScored("abcdefgh", "xy"),
		tokenScored("abcd", "z"),
	}
	res := newTokenStats(t, llm.HeuristicTokenCounter{}).Compute(scored)

	if got := res.Values["predicted_tokens_total"]; got != 3 {
		t.Errorf("predicted_tokens_total = %v, want 3", got)
	}
	if got := res.Values["reference_tokens_total"]; got != 2 {
		t.Errorf("reference_tokens_total = %v, want 2", got)
	}
	if got := res.Values["predicted_tokens_avg"]; got != 1.5 {
		t.Errorf("predicted_tokens_avg = %v, want 1.5", got)
	}
	if res.Value != res.Values["predicted_tokens_avg"] {
		t.Errorf("Value = %v, want it to equal predicted_tokens_avg %v", res.Value, res.Values["predicted_tokens_avg"])
	}
	if !res.Descriptive {
		t.Error("Descriptive = false, want true (TokenStats summarizes usage)")
	}
	if detail, ok := res.Detail.(map[string]string); !ok || detail["counter"] != "heuristic" {
		t.Errorf("Detail = %v, want counter=heuristic", res.Detail)
	}
}

// failingCounter fails to count any non-empty text, exercising the per-sample
// error path.
type failingCounter struct{}

func (failingCounter) Name() string { return "failing" }

func (failingCounter) Count(text string) (int, error) {
	if text == "" {
		return 0, nil
	}
	return 0, fmt.Errorf("failingCounter: cannot count %q", text)
}

func TestTokenStatsRecordsCounterErrors(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		tokenScored("abcd", "xy"),
		tokenScored("efgh", "z"),
	}
	res := newTokenStats(t, failingCounter{}).Compute(scored)

	// Every sample fails to count, so nothing is fabricated: totals stay 0 and
	// the failure count is surfaced rather than silently dropped.
	if got := res.Values["counter_errors"]; got != 2 {
		t.Errorf("counter_errors = %v, want 2", got)
	}
	if got := res.Values["predicted_tokens_total"]; got != 0 {
		t.Errorf("predicted_tokens_total = %v, want 0 (no fabricated counts)", got)
	}
	if res.Value != 0 {
		t.Errorf("Value = %v, want 0", res.Value)
	}
	detail, ok := res.Detail.(map[string]string)
	if !ok || detail["counter_errors"] != "2" {
		t.Errorf("Detail = %v, want counter_errors=2", res.Detail)
	}
}

// TokenStats produces a bench.RunMetric via the adapter, usable with the runner.
func TestTokenStatsAdaptsToRunMetric(t *testing.T) {
	t.Parallel()

	rm := AsRunMetric[string](newTokenStats(t, llm.HeuristicTokenCounter{}))
	out := rm.Compute([]bench.ScoredSample[string]{tokenScored("abcd", "abcd")})
	if out.Name != "token_stats[heuristic]" {
		t.Errorf("RunMetric name = %q", out.Name)
	}
	if !out.Descriptive {
		t.Error("RunMetric Descriptive = false, want true (propagated from Result)")
	}
}
