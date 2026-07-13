package provider_test

import (
	"context"
	"errors"
	"testing"
)

func TestIterator_ErrorOnClose(t *testing.T) {
	t.Parallel()
	closeErr := errors.New("close failed")
	iter := &countingIterator[int]{items: []int{1, 2}, closeErr: closeErr}

	// Read all items
	for {
		_, ok, err := iter.Next(context.Background())
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if !ok {
			break
		}
	}

	err := iter.Close()
	if !errors.Is(err, closeErr) {
		t.Fatalf("expected close error, got %v", err)
	}
}
