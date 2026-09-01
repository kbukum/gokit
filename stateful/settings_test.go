package stateful

import (
	"encoding/json"
	"testing"
	"time"
)

// TestAccumulatorSettingsGoldenJSON locks the cross-kit wire form of AccumulatorSettings and its
// tagged TriggerSpec variants: snake_case keys, ttl as a lossless duration string, and
// {"type":...} trigger tags shared byte-for-byte with the sibling kits.
func TestAccumulatorSettingsGoldenJSON(t *testing.T) {
	t.Parallel()

	settings := AccumulatorSettings{
		TTL:       30 * time.Second,
		KeepAlive: false,
		MaxSize:   100,
		Triggers: []TriggerSpec{
			SizeTriggerSpec(10),
			ByteSizeTriggerSpec(4096),
			TimeTriggerSpec(5 * time.Second),
		},
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"ttl":"30s","keep_alive":false,"max_size":100,"triggers":[` +
		`{"type":"size","threshold":10},` +
		`{"type":"byte_size","threshold":4096},` +
		`{"type":"time","interval":"5s"}]}`
	if string(raw) != want {
		t.Fatalf("settings JSON = %s, want %s", raw, want)
	}
}

// TestAccumulatorSettingsOmitsDefaults locks that a zero TTL and zero MaxSize are omitted and the
// triggers list is emitted as an empty array rather than null.
func TestAccumulatorSettingsOmitsDefaults(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(DefaultAccumulatorSettings())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const want = `{"keep_alive":true,"triggers":[]}`
	if string(raw) != want {
		t.Fatalf("default settings JSON = %s, want %s", raw, want)
	}
}

// TestAccumulatorSettingsKeepAliveDefault locks that keep_alive defaults to true when absent from
// the wire, matching the cross-kit default.
func TestAccumulatorSettingsKeepAliveDefault(t *testing.T) {
	t.Parallel()

	var out AccumulatorSettings
	if err := json.Unmarshal([]byte(`{"triggers":[]}`), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.KeepAlive {
		t.Errorf("keep_alive = false, want true when absent")
	}
}

// TestAccumulatorSettingsRoundTrip locks that settings survive a serde round-trip, including the
// TTL duration string and each tagged trigger variant.
func TestAccumulatorSettingsRoundTrip(t *testing.T) {
	t.Parallel()

	in := AccumulatorSettings{
		TTL:       90 * time.Second,
		KeepAlive: true,
		MaxSize:   256,
		Triggers: []TriggerSpec{
			SizeTriggerSpec(8),
			TimeTriggerSpec(2 * time.Minute),
		},
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out AccumulatorSettings
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.TTL != in.TTL || out.KeepAlive != in.KeepAlive || out.MaxSize != in.MaxSize ||
		len(out.Triggers) != len(in.Triggers) {
		t.Fatalf("round-trip = %+v, want %+v", out, in)
	}
	for i := range in.Triggers {
		if out.Triggers[i] != in.Triggers[i] {
			t.Errorf("trigger[%d] = %+v, want %+v", i, out.Triggers[i], in.Triggers[i])
		}
	}
}

// TestAccumulatorSettingsRejectsUnknownTrigger locks that an unknown trigger type is rejected on
// decode rather than silently ignored.
func TestAccumulatorSettingsRejectsUnknownTrigger(t *testing.T) {
	t.Parallel()

	err := json.Unmarshal([]byte(`{"triggers":[{"type":"bogus"}]}`), &AccumulatorSettings{})
	if err == nil {
		t.Fatal("expected error for unknown trigger type, got nil")
	}
}

// TestBuildConfigInstantiatesTriggers locks that BuildConfig carries TTL, keep-alive, and
// capacity through and builds one concrete Trigger per spec.
func TestBuildConfigInstantiatesTriggers(t *testing.T) {
	t.Parallel()

	settings := AccumulatorSettings{
		TTL:       time.Minute,
		KeepAlive: false,
		MaxSize:   50,
		Triggers: []TriggerSpec{
			SizeTriggerSpec(10),
			ByteSizeTriggerSpec(2048),
			TimeTriggerSpec(15 * time.Second),
		},
	}
	cfg, err := BuildConfig[string](settings)
	if err != nil {
		t.Fatalf("build config: %v", err)
	}
	if cfg.TTL != settings.TTL || cfg.KeepAlive != settings.KeepAlive || cfg.MaxSize != settings.MaxSize {
		t.Errorf("config scalars = %+v, want ttl=%v keepAlive=%v maxSize=%d",
			cfg, settings.TTL, settings.KeepAlive, settings.MaxSize)
	}
	if len(cfg.Triggers) != len(settings.Triggers) {
		t.Fatalf("built %d triggers, want %d", len(cfg.Triggers), len(settings.Triggers))
	}
}
