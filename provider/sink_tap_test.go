package provider_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/provider"
)

func TestTapSink_IsAvailableDelegates(t *testing.T) {
	var tapped []string
	sink := provider.TapSink[string](&collectSink{}, func(_ context.Context, in string) {
		tapped = append(tapped, in)
	})
	if !sink.IsAvailable(context.Background()) {
		t.Fatal("expected tap sink to be available")
	}
	if err := sink.Send(context.Background(), "x"); err != nil {
		t.Fatalf("send error: %v", err)
	}
	if len(tapped) != 1 || tapped[0] != "x" {
		t.Fatalf("expected tap to observe x, got %v", tapped)
	}
}
