package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAwait_NoTimeoutRunsDirectly(t *testing.T) {
	got, err := Await(context.Background(), 0, func(ctx context.Context) (int, error) {
		if _, ok := ctx.Deadline(); ok {
			t.Error("expected no deadline when timeout <= 0")
		}
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestAwait_WithTimeoutSetsDeadline(t *testing.T) {
	got, err := Await(context.Background(), time.Second, func(ctx context.Context) (string, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("expected deadline when timeout > 0")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("expected ok, got %q", got)
	}
}

func TestAwait_PropagatesError(t *testing.T) {
	sentinel := errors.New("boom")
	_, err := Await(context.Background(), time.Second, func(context.Context) (int, error) {
		return 0, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}
