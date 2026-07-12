package stateful

import (
	"context"
	"testing"
)

func TestByteSizeMeasurer_Accuracy(t *testing.T) {
	m := ByteSizeMeasurer()
	ctx := context.Background()

	values := [][]byte{
		[]byte(""),          // 0 bytes
		[]byte("hello"),     // 5 bytes
		[]byte("world!!!!"), // 9 bytes
	}

	result := m.Measure(ctx, values)
	if result != 14 {
		t.Errorf("expected 14 bytes, got %d", result)
	}

	// Empty slice → 0
	if m.Measure(ctx, nil) != 0 {
		t.Error("expected 0 for nil values")
	}
}

// ---------------------------------------------------------------------------
// Manager: TTL expiration during active use (keep-alive keeps it alive)
// ---------------------------------------------------------------------------
