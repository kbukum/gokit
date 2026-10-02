package sse

import (
	"context"
	"strings"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestFailureRejectsBeforeLargeEncodingAllocation(t *testing.T) {
	b := testBus(t, smallLimits())
	failure := apperrors.InvalidInput("payload", strings.Repeat("x", 2<<20))
	result := testing.Benchmark(func(bench *testing.B) {
		for range bench.N {
			if err := b.Fail(context.Background(), "scope", failure); err == nil {
				bench.Fatal("oversized failure accepted")
			}
		}
	})
	if result.AllocedBytesPerOp() > 64<<10 {
		t.Fatalf("rejecting 2 MiB failure allocated %d bytes/op; budget is 64 KiB", result.AllocedBytesPerOp())
	}
}

func TestOverflowDoesNotInterruptFastSubscriber(t *testing.T) {
	t.Parallel()
	b := testBus(t, smallLimits())
	slow := subscribe(t, b, "slow", "scope", "")
	fast := subscribe(t, b, "fast", "scope", "")
	publish(t, b, "scope", "one")
	next(t, fast)
	publish(t, b, "scope", "two")
	if e := next(t, fast); e.Name == "reset" || !strings.HasSuffix(e.ID, ":2") {
		t.Fatalf("fast subscriber lost: %+v", e)
	}
	if _, err := b.Subscribe(t.Context(), SubscribeRequest{Principal: "third", Route: "scope"}); err == nil {
		t.Fatal("terminal subscription escaped admission accounting")
	}
	if e := next(t, slow); e.Name != "reset" {
		t.Fatalf("slow subscriber not reset: %+v", e)
	}
	if b.Stats().Drops != 2 {
		t.Fatalf("dropped queued + triggering events: %+v", b.Stats())
	}
}
