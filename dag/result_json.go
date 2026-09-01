package dag

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/kbukum/gokit/dag/status"
)

// nodeResultWire is the cross-kit JSON shape of a NodeResult: a status string, a
// millisecond duration, and optional output/error. It mirrors the sibling kits so a graph
// result serialized by any kit decodes in the others.
type nodeResultWire struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms"`
	Output     any    `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
}

// MarshalJSON encodes the node result on the cross-kit wire form
// ({name, status, duration_ms, output?, error?}). Duration is emitted as whole
// milliseconds and any error as its message string; output and error are omitted when absent.
func (r NodeResult) MarshalJSON() ([]byte, error) {
	w := nodeResultWire{
		Name:       r.Name,
		Status:     r.Status.String(),
		DurationMs: r.Duration.Milliseconds(),
		Output:     r.Output,
	}
	if r.Error != nil {
		w.Error = r.Error.Error()
	}
	return json.Marshal(w)
}

// UnmarshalJSON decodes the cross-kit wire form produced by MarshalJSON back into a
// NodeResult. The status is validated against the canonical vocabulary, the millisecond
// duration is restored, and a non-empty error string is reconstructed as a plain error so
// the round trip preserves failure state. Output decodes to its generic JSON representation.
func (r *NodeResult) UnmarshalJSON(data []byte) error {
	var w nodeResultWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	st, err := status.Parse(w.Status)
	if err != nil {
		return err
	}
	r.Name = w.Name
	r.Status = st
	r.Duration = time.Duration(w.DurationMs) * time.Millisecond
	r.Output = w.Output
	if w.Error != "" {
		r.Error = errors.New(w.Error)
	} else {
		r.Error = nil
	}
	return nil
}

// resultWire is the cross-kit JSON shape of a graph Result: per-node results keyed by node
// id (serialized in deterministic sorted order by the encoder) and a total millisecond
// duration.
type resultWire struct {
	Nodes      map[string]NodeResult `json:"nodes"`
	DurationMs int64                 `json:"duration_ms"`
}

// MarshalJSON encodes the graph result on the cross-kit wire form
// ({nodes: {<id>: node}, duration_ms}).
func (r Result) MarshalJSON() ([]byte, error) {
	return json.Marshal(resultWire{
		Nodes:      r.NodeResults,
		DurationMs: r.Duration.Milliseconds(),
	})
}

// UnmarshalJSON decodes the cross-kit wire form produced by MarshalJSON back into a graph
// Result, restoring each node result (via NodeResult.UnmarshalJSON) and the total
// millisecond duration.
func (r *Result) UnmarshalJSON(data []byte) error {
	var w resultWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	r.NodeResults = w.Nodes
	r.Duration = time.Duration(w.DurationMs) * time.Millisecond
	return nil
}
