package codec_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kbukum/gokit/codec"
)

func TestJSONRoundTripsValue(t *testing.T) {
	t.Parallel()
	c := codec.PrettyJSON()
	value := map[string]any{"a": float64(1), "nested": map[string]any{"b": true}}

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
	if c.Name() != "json" {
		t.Fatalf("name: got %q", c.Name())
	}
}

func TestJSONPrettyMultilineCompactSingleLine(t *testing.T) {
	t.Parallel()
	value := map[string]any{"a": 1, "b": 2}

	pretty, err := codec.PrettyJSON().EncodeValue(value)
	if err != nil {
		t.Fatalf("pretty: %v", err)
	}
	if !strings.Contains(pretty, "\n") {
		t.Fatalf("pretty should be multiline: %q", pretty)
	}
	compact, err := codec.CompactJSON().EncodeValue(value)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if strings.Contains(compact, "\n") {
		t.Fatalf("compact should be single line: %q", compact)
	}
}

func TestJSONRejectsMalformedInput(t *testing.T) {
	t.Parallel()
	_, err := codec.PrettyJSON().DecodeValue("{ not json")
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestJSONRejectsUnrepresentableValue(t *testing.T) {
	t.Parallel()
	_, err := codec.PrettyJSON().EncodeValue(make(chan int))
	if err == nil || !strings.Contains(err.Error(), "serialize") {
		t.Fatalf("expected serialize error, got %v", err)
	}
}
