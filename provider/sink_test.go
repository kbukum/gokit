package provider_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/kbukum/gokit/provider"
)

// --- SinkFunc ---

func TestSinkFunc(t *testing.T) {
	var received []string
	sink := provider.NewSinkFunc("test", func(_ context.Context, s string) error {
		received = append(received, s)
		return nil
	})

	if sink.Name() != "test" {
		t.Errorf("expected name 'test', got %q", sink.Name())
	}
	if !sink.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable to return true")
	}

	ctx := context.Background()
	if err := sink.Send(ctx, "hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := sink.Send(ctx, "world"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(received) != 2 || received[0] != "hello" || received[1] != "world" {
		t.Errorf("expected [hello world], got %v", received)
	}
}

func TestSinkFunc_Error(t *testing.T) {
	sink := provider.NewSinkFunc("fail", func(_ context.Context, _ string) error {
		return fmt.Errorf("send failed")
	})

	err := sink.Send(context.Background(), "data")
	if err == nil || err.Error() != "send failed" {
		t.Errorf("expected 'send failed', got %v", err)
	}
}

// --- FanOutSink ---

func TestFanOutSink_Parallel(t *testing.T) {
	var mu sync.Mutex
	var log []string

	sink1 := provider.NewSinkFunc("s1", func(_ context.Context, s string) error {
		mu.Lock()
		log = append(log, "s1:"+s)
		mu.Unlock()
		return nil
	})
	sink2 := provider.NewSinkFunc("s2", func(_ context.Context, s string) error {
		mu.Lock()
		log = append(log, "s2:"+s)
		mu.Unlock()
		return nil
	})

	fan := provider.FanOutSink("fan", sink1, sink2)

	if fan.Name() != "fan" {
		t.Errorf("expected name 'fan', got %q", fan.Name())
	}

	if err := fan.Send(context.Background(), "msg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(log) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(log), log)
	}
	// Both sinks should have received the message (order may vary due to parallelism)
	has1, has2 := false, false
	for _, entry := range log {
		if entry == "s1:msg" {
			has1 = true
		}
		if entry == "s2:msg" {
			has2 = true
		}
	}
	if !has1 || !has2 {
		t.Errorf("expected both sinks to fire, got %v", log)
	}
}

func TestFanOutSink_ErrorsJoined(t *testing.T) {
	sink1 := provider.NewSinkFunc("ok", func(_ context.Context, _ string) error {
		return nil
	})
	sink2 := provider.NewSinkFunc("fail", func(_ context.Context, _ string) error {
		return fmt.Errorf("sink2 failed")
	})

	fan := provider.FanOutSink("fan", sink1, sink2)
	err := fan.Send(context.Background(), "msg")

	if err == nil {
		t.Fatal("expected error from failing sink")
	}
	if err.Error() != "sink2 failed" {
		t.Errorf("expected 'sink2 failed', got %q", err.Error())
	}
}

func TestFanOutSink_SingleSinkPassthrough(t *testing.T) {
	inner := provider.NewSinkFunc("inner", func(_ context.Context, _ string) error {
		return nil
	})

	fan := provider.FanOutSink("fan", inner)
	// Single sink should return the inner directly (optimization)
	if fan.Name() != "inner" {
		t.Errorf("expected passthrough to inner sink, got name %q", fan.Name())
	}
}

func TestFanOutSink_IsAvailable(t *testing.T) {
	available := &collectSink{}
	unavailable := &unavailableSink{}

	fan := provider.FanOutSink("fan", available, unavailable)
	// At least one is available → fan is available
	if !fan.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable=true when at least one sink is available")
	}
}

// --- AdaptSink ---

func TestAdaptSink(t *testing.T) {
	var received []string
	inner := provider.NewSinkFunc("inner", func(_ context.Context, s string) error {
		received = append(received, s)
		return nil
	})

	// Adapt int → string
	adapted := provider.AdaptSink(inner, "int-to-string",
		func(_ context.Context, n int) (string, error) {
			return fmt.Sprintf("num:%d", n), nil
		},
	)

	if adapted.Name() != "int-to-string" {
		t.Errorf("expected name 'int-to-string', got %q", adapted.Name())
	}

	if err := adapted.Send(context.Background(), 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(received) != 1 || received[0] != "num:42" {
		t.Errorf("expected [num:42], got %v", received)
	}
}

func TestAdaptSink_MapInError(t *testing.T) {
	inner := provider.NewSinkFunc("inner", func(_ context.Context, _ string) error {
		return nil
	})

	adapted := provider.AdaptSink(inner, "fail-map",
		func(_ context.Context, _ int) (string, error) {
			return "", fmt.Errorf("map failed")
		},
	)

	err := adapted.Send(context.Background(), 1)
	if err == nil || err.Error() != "map failed" {
		t.Errorf("expected 'map failed', got %v", err)
	}
}

// --- TapSink ---

func TestTapSink(t *testing.T) {
	var tapped []string
	var sent []string

	inner := provider.NewSinkFunc("inner", func(_ context.Context, s string) error {
		sent = append(sent, s)
		return nil
	})

	tap := provider.TapSink(inner, func(_ context.Context, s string) {
		tapped = append(tapped, s)
	})

	// Name delegates to inner
	if tap.Name() != "inner" {
		t.Errorf("expected name 'inner', got %q", tap.Name())
	}

	ctx := context.Background()
	if err := tap.Send(ctx, "a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := tap.Send(ctx, "b"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tapped) != 2 || tapped[0] != "a" || tapped[1] != "b" {
		t.Errorf("tap expected [a b], got %v", tapped)
	}
	if len(sent) != 2 || sent[0] != "a" || sent[1] != "b" {
		t.Errorf("send expected [a b], got %v", sent)
	}
}

// --- SinkMiddleware + ChainSink ---

func TestChainSink(t *testing.T) {
	var order []string

	mw1 := func(inner provider.Sink[string]) provider.Sink[string] {
		return provider.NewSinkFunc("mw1", func(ctx context.Context, s string) error {
			order = append(order, "mw1-before")
			err := inner.Send(ctx, s)
			order = append(order, "mw1-after")
			return err
		})
	}

	mw2 := func(inner provider.Sink[string]) provider.Sink[string] {
		return provider.NewSinkFunc("mw2", func(ctx context.Context, s string) error {
			order = append(order, "mw2-before")
			err := inner.Send(ctx, s)
			order = append(order, "mw2-after")
			return err
		})
	}

	inner := provider.NewSinkFunc("inner", func(_ context.Context, _ string) error {
		order = append(order, "inner")
		return nil
	})

	chain := provider.ChainSink(
		provider.SinkMiddleware[string](mw1),
		provider.SinkMiddleware[string](mw2),
	)
	wrapped := chain(inner)

	if err := wrapped.Send(context.Background(), "x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Chain(mw1, mw2)(inner) = mw1(mw2(inner))
	// Execution: mw1-before → mw2-before → inner → mw2-after → mw1-after
	expected := []string{"mw1-before", "mw2-before", "inner", "mw2-after", "mw1-after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("position %d: expected %q, got %q", i, v, order[i])
		}
	}
}

// --- test helpers ---

type unavailableSink struct{}

func (s *unavailableSink) Name() string                           { return "unavailable" }
func (s *unavailableSink) IsAvailable(_ context.Context) bool     { return false }
func (s *unavailableSink) Send(_ context.Context, _ string) error { return nil }
