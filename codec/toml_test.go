package codec_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kbukum/gokit/codec"
)

func TestTOMLRoundTripsTable(t *testing.T) {
	t.Parallel()
	c := codec.NewTOMLCodec()
	// Number-free tree so the value round trip is exact: TOML re-types numeric
	// scalars, which the typed Encode/Decode path normalizes but a raw value
	// comparison does not.
	value := map[string]any{
		"name":   "svc",
		"nested": map[string]any{"enabled": true},
	}

	encoded, err := c.EncodeValue(value)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := c.DecodeValue(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(decoded, value) {
		t.Fatalf("round trip: got %#v want %#v", decoded, value)
	}
	if c.Name() != "toml" {
		t.Fatalf("name: got %q", c.Name())
	}
}

func TestTOMLRejectsMalformedInput(t *testing.T) {
	t.Parallel()
	_, err := codec.NewTOMLCodec().DecodeValue("= not valid")
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestTOMLRejectsNilValue(t *testing.T) {
	t.Parallel()
	_, err := codec.NewTOMLCodec().EncodeValue(nil)
	if err == nil || !strings.Contains(err.Error(), "serialize") {
		t.Fatalf("expected serialize error, got %v", err)
	}
}
