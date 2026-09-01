package stateful

import (
	"encoding/json"
	"testing"
	"time"
)

// TestStateSnapshotGoldenJSON locks the snake_case wire field names so the snapshot stays
// byte-interchangeable with the sibling kits.
func TestStateSnapshotGoldenJSON(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(StateSnapshot[string]{State: "submitted", Version: 7})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"state":"submitted","version":7}`
	if string(raw) != want {
		t.Fatalf("snapshot JSON = %s, want %s", raw, want)
	}
}

// TestAuditEntryGoldenJSON locks the snake_case wire field names and the RFC 3339 timestamp
// encoding shared across kits.
func TestAuditEntryGoldenJSON(t *testing.T) {
	t.Parallel()

	type ctx struct {
		Actor string `json:"actor"`
	}
	in := AuditEntry[string, ctx]{
		Transition: "submit",
		From:       "draft",
		To:         "submitted",
		Context:    ctx{Actor: "alice"},
		Version:    3,
		RecordedAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"transition":"submit","from":"draft","to":"submitted",` +
		`"context":{"actor":"alice"},"version":3,"recorded_at":"2024-01-02T03:04:05Z"}`
	if string(raw) != want {
		t.Fatalf("audit JSON = %s, want %s", raw, want)
	}
}

// TestStateSnapshotRoundTrip locks that a StateSnapshot survives a serde round-trip
// so it stays aligned across kits.
func TestStateSnapshotRoundTrip(t *testing.T) {
	t.Parallel()

	in := StateSnapshot[string]{State: "submitted", Version: 7}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out StateSnapshot[string]
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Errorf("round-trip = %+v, want %+v", out, in)
	}
}

// TestAuditEntryRoundTrip locks that an AuditEntry survives a serde round-trip,
// including its wall-clock timestamp and typed context.
func TestAuditEntryRoundTrip(t *testing.T) {
	t.Parallel()

	type ctx struct {
		Actor string `json:"actor"`
	}
	in := AuditEntry[string, ctx]{
		Transition: "submit",
		From:       "draft",
		To:         "submitted",
		Context:    ctx{Actor: "alice"},
		Version:    3,
		RecordedAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out AuditEntry[string, ctx]
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Transition != in.Transition || out.From != in.From || out.To != in.To ||
		out.Context != in.Context || out.Version != in.Version || !out.RecordedAt.Equal(in.RecordedAt) {
		t.Errorf("round-trip = %+v, want %+v", out, in)
	}
}
