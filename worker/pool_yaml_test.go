package worker_test

import (
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"go.yaml.in/yaml/v3"

	"github.com/kbukum/gokit/worker"
)

func TestPoolConfigOverflowYAMLDecodeDropOldest(t *testing.T) {
	t.Parallel()

	cfg := decodePoolConfigYAML(t, []byte("name: yaml-drop\noverflow: drop_oldest\n"))
	if cfg.Overflow != worker.OverflowDropOldest {
		t.Fatalf("overflow = %q, want %q", cfg.Overflow, worker.OverflowDropOldest)
	}
}

func TestPoolConfigOverflowYAMLDecodeReject(t *testing.T) {
	t.Parallel()

	cfg := decodePoolConfigYAML(t, []byte("name: yaml-reject\noverflow: reject\n"))
	if cfg.Overflow != worker.OverflowReject {
		t.Fatalf("overflow = %q, want %q", cfg.Overflow, worker.OverflowReject)
	}
}

func TestPoolConfigOverflowYAMLDecodeInvalid(t *testing.T) {
	t.Parallel()

	var raw map[string]any
	if err := yaml.Unmarshal([]byte("overflow: newest\n"), &raw); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	var cfg worker.PoolConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.TextUnmarshallerHookFunc(),
		Result:     &cfg,
		TagName:    "mapstructure",
	})
	if err != nil {
		t.Fatalf("new decoder: %v", err)
	}
	if err := decoder.Decode(raw); err == nil {
		t.Fatal("expected invalid overflow policy error")
	}
}

func decodePoolConfigYAML(t *testing.T, blob []byte) worker.PoolConfig {
	t.Helper()
	var raw map[string]any
	if err := yaml.Unmarshal(blob, &raw); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	var cfg worker.PoolConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.TextUnmarshallerHookFunc(),
		Result:     &cfg,
		TagName:    "mapstructure",
	})
	if err != nil {
		t.Fatalf("new decoder: %v", err)
	}
	if err := decoder.Decode(raw); err != nil {
		t.Fatalf("decode pool config: %v", err)
	}
	return cfg
}
