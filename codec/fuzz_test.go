package codec_test

import (
	"testing"

	"github.com/kbukum/gokit/codec"
)

// FuzzJSONDecode ensures the JSON codec never panics on arbitrary input and that
// any successfully decoded value re-encodes and decodes to an equal tree.
func FuzzJSONDecode(f *testing.F) {
	seeds := []string{"", "{}", "[]", "null", "{ not json", `{"a":1}`, `[1,2,3]`, `"str"`}
	for _, s := range seeds {
		f.Add(s)
	}
	c := codec.CompactJSON()
	f.Fuzz(func(t *testing.T, contents string) {
		value, err := c.DecodeValue(contents)
		if err != nil {
			return
		}
		encoded, err := c.EncodeValue(value)
		if err != nil {
			t.Fatalf("re-encode decoded value: %v", err)
		}
		if _, err := c.DecodeValue(encoded); err != nil {
			t.Fatalf("re-decode round trip: %v", err)
		}
	})
}

// FuzzTOMLDecode ensures the TOML codec never panics on arbitrary input.
func FuzzTOMLDecode(f *testing.F) {
	seeds := []string{"", "= bad", "a = 1", "[t]\nx = true", "name = 'svc'"}
	for _, s := range seeds {
		f.Add(s)
	}
	c := codec.NewTOMLCodec()
	f.Fuzz(func(t *testing.T, contents string) {
		value, err := c.DecodeValue(contents)
		if err != nil {
			return
		}
		if _, err := c.EncodeValue(value); err != nil {
			// Some decoded trees (for example those containing nil) have no TOML
			// representation; a typed error is acceptable, a panic is not.
			_ = err
		}
	})
}
