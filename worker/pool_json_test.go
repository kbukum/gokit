package worker

import (
	"encoding/json"
	"testing"
	"time"
)

// TestPoolConfigGoldenJSON locks the cross-kit wire form of a PoolConfig: snake_case keys, the
// grace period as a lossless duration string, and snake_case dispatch/overflow enums. It stays
// byte-interchangeable with the sibling kits.
func TestPoolConfigGoldenJSON(t *testing.T) {
	t.Parallel()

	cfg := PoolConfig{
		Name:        "ingest",
		Size:        8,
		QueueSize:   256,
		EventBuffer: 64,
		GracePeriod: 3601 * time.Second,
		Dispatch:    RoundRobin,
		Overflow:    OverflowReject,
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"name":"ingest","size":8,"queue_size":256,"event_buffer":64,` +
		`"grace_period":"3601s","dispatch":"round_robin","overflow":"reject"}`
	if string(raw) != want {
		t.Fatalf("pool config JSON = %s, want %s", raw, want)
	}
}

// TestPoolConfigRoundTrip locks that a PoolConfig survives a serde round-trip, including the
// grace-period duration string.
func TestPoolConfigRoundTrip(t *testing.T) {
	t.Parallel()

	in := PoolConfig{
		Name:        "ingest",
		Size:        4,
		QueueSize:   128,
		EventBuffer: 32,
		GracePeriod: 90 * time.Second,
		Dispatch:    LeastLoaded,
		Overflow:    OverflowDropOldest,
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out PoolConfig
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Errorf("round-trip = %+v, want %+v", out, in)
	}
}

// TestPoolConfigRoundTripWithSupervisor locks that the nested supervisor configuration also
// round-trips, with its durations encoded as lossless strings.
func TestPoolConfigRoundTripWithSupervisor(t *testing.T) {
	t.Parallel()

	in := PoolConfig{
		Name:        "ingest",
		Size:        4,
		QueueSize:   128,
		EventBuffer: 32,
		GracePeriod: 5 * time.Second,
		Dispatch:    RoundRobin,
		Overflow:    OverflowBlock,
		Supervisor: &SupervisorConfig{
			RestartPolicy:  RestartOnFailure,
			MaxRestarts:    3,
			BackoffBase:    time.Second,
			HealthInterval: 30 * time.Second,
		},
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out PoolConfig
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Supervisor == nil || *out.Supervisor != *in.Supervisor {
		t.Fatalf("round-trip supervisor = %+v, want %+v", out.Supervisor, in.Supervisor)
	}
	out.Supervisor, in.Supervisor = nil, nil
	if out != in {
		t.Errorf("round-trip = %+v, want %+v", out, in)
	}
}
