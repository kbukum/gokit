package payload

import (
	"testing"
)

func TestLimitsWithDefaults(t *testing.T) {
	t.Parallel()
	got := Limits{}.WithDefaults()
	if got.MaxInMemoryBytes != DefaultMaxInMemoryBytes {
		t.Errorf("MaxInMemoryBytes = %d; want %d", got.MaxInMemoryBytes, DefaultMaxInMemoryBytes)
	}
	if got.StreamBuffer != DefaultStreamBuffer {
		t.Errorf("StreamBuffer = %d; want %d", got.StreamBuffer, DefaultStreamBuffer)
	}

	custom := Limits{MaxInMemoryBytes: 10, StreamBuffer: 2}.WithDefaults()
	if custom.MaxInMemoryBytes != 10 || custom.StreamBuffer != 2 {
		t.Errorf("WithDefaults overrode explicit values: %+v", custom)
	}
}

func TestDefaultLimits(t *testing.T) {
	t.Parallel()
	l := DefaultLimits()
	if l.MaxInMemoryBytes != DefaultMaxInMemoryBytes || l.StreamBuffer != DefaultStreamBuffer {
		t.Fatalf("DefaultLimits = %+v", l)
	}
}
