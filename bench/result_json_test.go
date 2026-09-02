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

// TestRunResultStoredSchemaEnvelope locks the stored-run envelope: a persisted RunResult carries
// the canonical `$schema` URL and `version` at the top level (matching the report shape and the
// sibling rskit BenchRunResult), and never the legacy bare `schema` key.
func TestRunResultStoredSchemaEnvelope(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(RunResult{ID: "run-1", Duration: time.Second})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if got := string(envelope["$schema"]); got != `"`+SchemaURL+`"` {
		t.Errorf("$schema = %s, want %q", got, SchemaURL)
	}
	if got := string(envelope["version"]); got != `"`+SchemaVersion+`"` {
		t.Errorf("version = %s, want %q", got, SchemaVersion)
	}
	if _, ok := envelope["schema"]; ok {
		t.Errorf("stored run JSON still emits a bare \"schema\" key: %s", raw)
	}
}

// TestRunResultDecodesCrossKitEnvelope locks that a stored run written with the `$schema`/`version`
// envelope round-trips: the envelope keys are accepted and the payload restores intact.
func TestRunResultDecodesCrossKitEnvelope(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(RunResult{ID: "run-1", Tag: "nightly", Duration: 1500 * time.Millisecond})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out RunResult
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ID != "run-1" || out.Tag != "nightly" || out.Duration != 1500*time.Millisecond {
		t.Errorf("round-trip = %+v, want id=run-1 tag=nightly duration=1.5s", out)
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
