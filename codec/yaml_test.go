package codec

import (
	"strings"
	"testing"
)

func TestYAMLCodecRoundTrip(t *testing.T) {
	t.Parallel()

	codec := NewYAMLCodec()
	value := map[string]any{
		"name": "svc",
		"tags": []any{"alpha", "beta", "gamma"},
		"nested": map[string]any{
			"enabled": true,
			"retries": int64(3),
		},
	}

	encoded, err := codec.EncodeValue(value)
	if err != nil {
		t.Fatalf("EncodeValue: %v", err)
	}
	decoded, err := codec.DecodeValue(encoded)
	if err != nil {
		t.Fatalf("DecodeValue: %v", err)
	}
	m, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("decoded is %T, want map", decoded)
	}
	if m["name"] != "svc" {
		t.Errorf("name = %v, want svc", m["name"])
	}
	if codec.Name() != "yaml" {
		t.Errorf("Name = %q, want yaml", codec.Name())
	}
}

func TestYAMLCodecRejectsMalformed(t *testing.T) {
	t.Parallel()

	_, err := NewYAMLCodec().DecodeValue("key: [unclosed")
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestYAMLCodecRejectsNonMappingDecode(t *testing.T) {
	t.Parallel()

	for _, doc := range []string{"- a\n- b", "42", ""} {
		_, err := NewYAMLCodec().DecodeValue(doc)
		if err == nil || !strings.Contains(err.Error(), "mapping") {
			t.Errorf("doc %q: expected mapping error, got %v", doc, err)
		}
	}
}

func TestYAMLCodecRejectsNonMappingEncode(t *testing.T) {
	t.Parallel()

	_, err := NewYAMLCodec().EncodeValue(nil)
	if err == nil || !strings.Contains(err.Error(), "mapping") {
		t.Fatalf("expected mapping error, got %v", err)
	}
}

func TestCodecForNameYAML(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"yaml", "YAML", "yml"} {
		c, ok := CodecForName(name)
		if !ok || c.Name() != "yaml" {
			t.Errorf("CodecForName(%q) = (%v,%v)", name, c, ok)
		}
	}
	if c, ok := CodecForPath("/etc/app/config.yaml"); !ok || c.Name() != "yaml" {
		t.Errorf("CodecForPath yaml = (%v,%v)", c, ok)
	}
}
