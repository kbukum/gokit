package dag_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/kbukum/gokit/dag"
	"github.com/kbukum/gokit/dag/status"
)

// TestResultGoldenJSON locks the cross-kit wire contract for a graph result: a nodes map
// keyed by node id in sorted order, snake_case status strings, millisecond durations, and
// output/error omitted when absent.
func TestResultGoldenJSON(t *testing.T) {
	t.Parallel()

	result := dag.Result{
		Duration: 20 * time.Millisecond,
		NodeResults: map[string]dag.NodeResult{
			"load": {
				Name:     "load",
				Status:   status.Completed,
				Duration: 12 * time.Millisecond,
				Output:   map[string]any{"rows": 3},
			},
			"transform": {
				Name:     "transform",
				Status:   status.Failed,
				Duration: 4 * time.Millisecond,
				Error:    errors.New("boom"),
			},
			"publish": {
				Name:   "publish",
				Status: status.Skipped,
			},
		},
	}

	got, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"nodes":{` +
		`"load":{"name":"load","status":"completed","duration_ms":12,"output":{"rows":3}},` +
		`"publish":{"name":"publish","status":"skipped","duration_ms":0},` +
		`"transform":{"name":"transform","status":"failed","duration_ms":4,"error":"boom"}` +
		`},"duration_ms":20}`
	if string(got) != want {
		t.Errorf("Result JSON =\n%s\nwant\n%s", got, want)
	}
}

// TestNodeResultRicherStatusSerializes confirms gokit's richer skip-reason vocabulary
// serializes as its wire string (a superset of the shared status set).
func TestNodeResultRicherStatusSerializes(t *testing.T) {
	t.Parallel()

	got, err := json.Marshal(dag.NodeResult{
		Name:   "consumer",
		Status: status.DepFailed,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"name":"consumer","status":"skipped:dep_failed","duration_ms":0}`
	if string(got) != want {
		t.Errorf("NodeResult JSON = %s, want %s", got, want)
	}
}

// TestResultRoundTrip confirms a graph result survives a marshal/unmarshal cycle: node
// results, statuses, millisecond durations, output, and error message are all restored.
func TestResultRoundTrip(t *testing.T) {
	t.Parallel()

	original := dag.Result{
		Duration: 20 * time.Millisecond,
		NodeResults: map[string]dag.NodeResult{
			"load": {
				Name:     "load",
				Status:   status.Completed,
				Duration: 12 * time.Millisecond,
				Output:   map[string]any{"rows": float64(3)},
			},
			"transform": {
				Name:     "transform",
				Status:   status.Failed,
				Duration: 4 * time.Millisecond,
				Error:    errors.New("boom"),
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded dag.Result
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Duration != original.Duration {
		t.Errorf("Duration = %v, want %v", decoded.Duration, original.Duration)
	}
	if len(decoded.NodeResults) != len(original.NodeResults) {
		t.Fatalf("NodeResults len = %d, want %d", len(decoded.NodeResults), len(original.NodeResults))
	}
	load := decoded.NodeResults["load"]
	if load.Name != "load" || load.Status != status.Completed || load.Duration != 12*time.Millisecond {
		t.Errorf("load node = %+v", load)
	}
	if !reflect.DeepEqual(load.Output, map[string]any{"rows": float64(3)}) {
		t.Errorf("load output = %#v", load.Output)
	}
	transform := decoded.NodeResults["transform"]
	if transform.Status != status.Failed || transform.Error == nil || transform.Error.Error() != "boom" {
		t.Errorf("transform node = %+v (err=%v)", transform, transform.Error)
	}
}

// TestNodeResultUnmarshalRejectsUnknownStatus confirms decoding fails for a status outside
// the canonical vocabulary rather than silently accepting an invalid value.
func TestNodeResultUnmarshalRejectsUnknownStatus(t *testing.T) {
	t.Parallel()

	var nr dag.NodeResult
	err := json.Unmarshal([]byte(`{"name":"x","status":"bogus","duration_ms":1}`), &nr)
	if err == nil {
		t.Fatal("expected error for unknown status, got nil")
	}
}
