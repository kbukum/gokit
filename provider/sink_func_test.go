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

func TestSinkFunc_NilFunction_NotAvailable(t *testing.T) {
	t.Parallel()
	// A nil-fn sink can never succeed, so it must report unavailable to
	// selectors and health checks.
	sink := provider.NewSinkFunc[string]("nil-fn", nil)
	if sink.IsAvailable(context.Background()) {
		t.Fatal("nil-fn SinkFunc must report IsAvailable() == false")
	}
}

func TestSinkFunc_Available(t *testing.T) {
	t.Parallel()
	sink := provider.NewSinkFunc[string]("ok", func(context.Context, string) error { return nil })
	if !sink.IsAvailable(context.Background()) {
		t.Fatal("SinkFunc with a non-nil fn must report IsAvailable() == true")
	}
}
