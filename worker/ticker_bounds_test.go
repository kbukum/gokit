package worker_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kbukum/gokit/worker"
)

func TestTickerStopHonorsCallerDeadline(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		ticker := worker.NewTickerWorker("bounded", time.Hour, func(context.Context) error {
			close(started)
			time.Sleep(time.Second)
			return nil
		}, worker.WithRunOnStart())
		if err := ticker.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		<-started
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		defer cancel()
		err := ticker.Stop(ctx)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("stop exceeded its deadline without reporting it: %v", err)
		}
		if err := ticker.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestTickerCanceledStartupDoesNotRun(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	ticker := worker.NewTickerWorker("canceled", time.Hour, func(context.Context) error { return nil })
	err := ticker.Start(ctx)
	if err := ticker.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup accepted: %v", err)
	}
}
