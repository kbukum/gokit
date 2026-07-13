package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/provider"
)

func TestSinkFunc_NilFunction_FailsClosed(t *testing.T) {
	t.Parallel()
	// A SinkFunc built with a nil fn must fail closed with a typed error
	// on Send rather than panicking on the runtime path.
	sink := provider.NewSinkFunc[string]("nil-fn", nil)
	err := sink.Send(context.Background(), "test")
	if !errors.Is(err, provider.ErrNilSinkFunc) {
		t.Fatalf("expected ErrNilSinkFunc, got %v", err)
	}
}
