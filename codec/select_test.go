package codec_test

import (
	"testing"

	"github.com/kbukum/gokit/codec"
)

func TestCodecForNameAndPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want string
	}{
		{"json", "json"},
		{"JSON", "json"},
		{"toml", "toml"},
		{"TOML", "toml"},
	}
	for _, tc := range tests {
		c, ok := codec.CodecForName(tc.name)
		if !ok || c.Name() != tc.want {
			t.Fatalf("CodecForName(%q): got (%v, %t)", tc.name, c, ok)
		}
	}

	c, ok := codec.CodecForPath("/etc/app/config.json")
	if !ok || c.Name() != "json" {
		t.Fatalf("CodecForPath json: got (%v, %t)", c, ok)
	}
	c, ok = codec.CodecForPath("config.toml")
	if !ok || c.Name() != "toml" {
		t.Fatalf("CodecForPath toml: got (%v, %t)", c, ok)
	}
}

func TestCodecForUnknownOrMissing(t *testing.T) {
	t.Parallel()
	if _, ok := codec.CodecForName("yaml"); ok {
		t.Fatal("yaml should be unknown")
	}
	if _, ok := codec.CodecForPath("config"); ok {
		t.Fatal("extensionless path should be unknown")
	}
}
