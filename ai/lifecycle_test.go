package ai_test

import (
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/ai"
)

func TestLifecycleReadyTransitions(t *testing.T) {
	t.Parallel()
	var lc ai.Lifecycle

	if lc.Ready() {
		t.Fatal("zero-value lifecycle must not be ready")
	}
	if !lc.LastCall().IsZero() {
		t.Fatalf("zero-value LastCall must be zero, got %v", lc.LastCall())
	}

	lc.MarkReady()
	if !lc.Ready() {
		t.Fatal("MarkReady must make the lifecycle ready")
	}

	lc.MarkStopped()
	if lc.Ready() {
		t.Fatal("MarkStopped must clear readiness")
	}

	// A restart returns to ready (stopped flag cleared).
	lc.MarkReady()
	if !lc.Ready() {
		t.Fatal("MarkReady after stop must restore readiness")
	}
}

func TestLifecycleTouchRecordsLastCall(t *testing.T) {
	t.Parallel()
	var lc ai.Lifecycle

	before := time.Now()
	lc.Touch()
	last := lc.LastCall()
	if last.Before(before) {
		t.Fatalf("Touch must record a timestamp at or after the call, got %v < %v", last, before)
	}
}

// TestLifecycleConcurrentAccess proves the mixin is race-free under concurrent
// readers and writers (run under -race).
func TestLifecycleConcurrentAccess(t *testing.T) {
	t.Parallel()
	var lc ai.Lifecycle
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				lc.MarkReady()
				lc.Touch()
			} else {
				lc.MarkStopped()
			}
			_ = lc.Ready()
			_ = lc.LastCall()
		}(i)
	}
	wg.Wait()
}
