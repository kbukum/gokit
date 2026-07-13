package provider_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/provider"
)

func TestSinkFunc_NilFunction_Panics(t *testing.T) {
	t.Parallel()
	// Creating a SinkFunc with nil fn should panic when Send is called
	sink := provider.NewSinkFunc[string]("nil-fn", nil)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when calling Send with nil function")
		}
	}()
	_ = sink.Send(context.Background(), "test")
}
