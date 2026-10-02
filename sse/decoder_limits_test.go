package sse

import (
	"strings"
	"testing"
)

func TestDecoderBoundsMultilineFrames(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		wire  string
		valid bool
	}{
		{"data:a\ndata: b\n\n", true},
		{"data:a\ndata: bc\n\n", false},
		{strings.Repeat("data:\n", 100), false},
		{": keepalive\n\n: keepalive\n\ndata:a\ndata: b\n\n", true},
	} {
		decoder := NewDecoder(strings.NewReader(tc.wire), WithMaxFrameSize(16))
		_, err := decoder.Next()
		if (err == nil) != tc.valid {
			t.Fatalf("frame %q valid=%v: %v", tc.wire, tc.valid, err)
		}
	}
}
