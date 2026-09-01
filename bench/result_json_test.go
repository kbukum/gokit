package bench

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestRunResultDurationMillis locks that RunResult emits its elapsed time as whole milliseconds
// under duration_ms (not the default nanosecond integer) and round-trips.
func TestRunResultDurationMillis(t *testing.T) {
	t.Parallel()

	in := RunResult{ID: "run-1", Duration: 1500 * time.Millisecond}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"duration_ms":1500`) {
		t.Fatalf("run result JSON = %s, want duration_ms=1500", raw)
	}

	var out RunResult
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Duration != in.Duration {
		t.Errorf("round-trip duration = %v, want %v", out.Duration, in.Duration)
	}
}

// TestBranchResultDurationMillis locks the millisecond duration_ms encoding for BranchResult.
func TestBranchResultDurationMillis(t *testing.T) {
	t.Parallel()

	in := BranchResult{Name: "fast", Duration: 250 * time.Millisecond}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"duration_ms":250`) {
		t.Fatalf("branch result JSON = %s, want duration_ms=250", raw)
	}

	var out BranchResult
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Duration != in.Duration {
		t.Errorf("round-trip duration = %v, want %v", out.Duration, in.Duration)
	}
}

// TestSampleResultDurationMillis locks the millisecond duration_ms encoding for SampleResult.
func TestSampleResultDurationMillis(t *testing.T) {
	t.Parallel()

	in := SampleResult{ID: "s-1", Duration: 42 * time.Millisecond}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"duration_ms":42`) {
		t.Fatalf("sample result JSON = %s, want duration_ms=42", raw)
	}

	var out SampleResult
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Duration != in.Duration {
		t.Errorf("round-trip duration = %v, want %v", out.Duration, in.Duration)
	}
}
