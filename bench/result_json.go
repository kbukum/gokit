package bench

import (
	"encoding/json"
	"time"
)

// The benchmark result contract carries every elapsed time under a `duration_ms` key measured
// in whole milliseconds, matching the sibling kits so a run serialized by any kit decodes in the
// others. Duration fields are stored as time.Duration in memory, so each result type overrides
// JSON (un)marshaling to emit and restore the millisecond form rather than the default
// nanosecond integer.

// MarshalJSON encodes the run result with its Duration as whole milliseconds under duration_ms.
func (r RunResult) MarshalJSON() ([]byte, error) {
	type alias RunResult
	return json.Marshal(struct {
		alias
		Duration int64 `json:"duration_ms"`
	}{alias: alias(r), Duration: r.Duration.Milliseconds()})
}

// UnmarshalJSON decodes the run result, restoring Duration from the millisecond duration_ms field.
func (r *RunResult) UnmarshalJSON(data []byte) error {
	type alias RunResult
	aux := struct {
		*alias
		Duration int64 `json:"duration_ms"`
	}{alias: (*alias)(r)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	r.Duration = time.Duration(aux.Duration) * time.Millisecond
	return nil
}

// MarshalJSON encodes the branch result with its Duration as whole milliseconds under duration_ms.
func (b BranchResult) MarshalJSON() ([]byte, error) {
	type alias BranchResult
	return json.Marshal(struct {
		alias
		Duration int64 `json:"duration_ms"`
	}{alias: alias(b), Duration: b.Duration.Milliseconds()})
}

// UnmarshalJSON decodes the branch result, restoring Duration from the millisecond duration_ms field.
func (b *BranchResult) UnmarshalJSON(data []byte) error {
	type alias BranchResult
	aux := struct {
		*alias
		Duration int64 `json:"duration_ms"`
	}{alias: (*alias)(b)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	b.Duration = time.Duration(aux.Duration) * time.Millisecond
	return nil
}

// MarshalJSON encodes the sample result with its Duration as whole milliseconds under duration_ms.
func (s SampleResult) MarshalJSON() ([]byte, error) {
	type alias SampleResult
	return json.Marshal(struct {
		alias
		Duration int64 `json:"duration_ms"`
	}{alias: alias(s), Duration: s.Duration.Milliseconds()})
}

// UnmarshalJSON decodes the sample result, restoring Duration from the millisecond duration_ms field.
func (s *SampleResult) UnmarshalJSON(data []byte) error {
	type alias SampleResult
	aux := struct {
		*alias
		Duration int64 `json:"duration_ms"`
	}{alias: (*alias)(s)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	s.Duration = time.Duration(aux.Duration) * time.Millisecond
	return nil
}
