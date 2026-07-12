package framing_test

import (
	"bytes"
	"testing"

	"github.com/kbukum/gokit/codec/framing"
)

// FuzzReadFrame ensures the framed reader never panics or over-allocates on
// arbitrary bytes, and that any frame written and read back round-trips.
func FuzzReadFrame(f *testing.F) {
	f.Add([]byte(nil))
	f.Add([]byte{0, 0, 0, 1, 'x'})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 'x'})
	f.Fuzz(func(t *testing.T, data []byte) {
		// A tight cap ensures a hostile length prefix is rejected, not allocated.
		_, _ = framing.ReadFrame(bytes.NewReader(data), 1024)

		if len(data) <= framing.DefaultMaxFrameBytes {
			var buf bytes.Buffer
			if err := framing.WriteFrame(&buf, data, framing.DefaultMaxFrameBytes); err != nil {
				t.Fatalf("write: %v", err)
			}
			got, err := framing.ReadFrame(&buf, framing.DefaultMaxFrameBytes)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("round trip mismatch: got %v want %v", got, data)
			}
		}
	})
}
