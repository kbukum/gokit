package chain_test

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/chain"
)

// TestStepProgressGoldenJSON locks the cross-kit wire contract for StepProgress:
// snake_case keys, status vocabulary, and message omitted when empty.
func TestStepProgressGoldenJSON(t *testing.T) {
	t.Parallel()

	got, err := json.Marshal(chain.StepProgress{
		StepIndex:       2,
		StepID:          "parse",
		Status:          chain.StatusRunning,
		ProgressPercent: 50,
		Message:         "halfway",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"step_index":2,"step_id":"parse","status":"running","progress_percent":50,"message":"halfway"}`
	if string(got) != want {
		t.Errorf("StepProgress JSON = %s, want %s", got, want)
	}
}

// TestStepProgressOmitsEmptyMessage confirms message is omitted when unset.
func TestStepProgressOmitsEmptyMessage(t *testing.T) {
	t.Parallel()

	got, err := json.Marshal(chain.StepProgress{
		StepIndex:       0,
		StepID:          "done",
		Status:          chain.StatusCompleted,
		ProgressPercent: 100,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"step_index":0,"step_id":"done","status":"completed","progress_percent":100}`
	if string(got) != want {
		t.Errorf("StepProgress JSON = %s, want %s", got, want)
	}
}
