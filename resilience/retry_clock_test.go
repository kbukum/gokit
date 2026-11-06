package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/util"
)

func TestRetry_UsesInjectedClockForBackoff(t *testing.T) {
	clock := util.NewFakeClock(time.Unix(0, 0))
	cfg := RetryConfig{MaxAttempts: 2, InitialBackoff: time.Hour, MaxBackoff: time.Hour, Strategy: ConstantBackoff, Clock: clock}
	started := make(chan struct{}, 2)
	done := make(chan error, 1)
	go func() {
		_, err := Retry(context.Background(), cfg, func() (int, error) {
			started <- struct{}{}
			return 0, errors.New("transient")
		})
		done <- err
	}()
	<-started
	for {
		select {
		case <-started:
			if err := <-done; !errors.Is(err, ErrMaxRetriesExceeded) {
				t.Fatalf("Retry() error = %v, want ErrMaxRetriesExceeded", err)
			}
			return
		default:
			clock.Advance(time.Hour)
			time.Sleep(time.Millisecond)
		}
	}
}

func TestRetry_InjectedClockBoundsElapsedTime(t *testing.T) {
	clock := util.NewFakeClock(time.Unix(0, 0))
	cfg := RetryConfig{MaxAttempts: 5, InitialBackoff: time.Millisecond, MaxElapsedTime: time.Minute, Clock: clock}
	calls := 0
	_, err := Retry(context.Background(), cfg, func() (int, error) {
		calls++
		clock.Advance(2 * time.Minute)
		return 0, errors.New("slow")
	})
	if !errors.Is(err, ErrMaxRetriesExceeded) || calls != 1 {
		t.Fatalf("Retry() = %v after %d calls, want budget stop after 1", err, calls)
	}
}

func TestRetryTreatsTypedNilClockAsUnset(t *testing.T) {
	t.Parallel()
	var clock *util.FakeClock
	calls := 0
	_, err := Retry(t.Context(), RetryConfig{MaxAttempts: 2, InitialBackoff: time.Millisecond, Clock: clock}, func() (int, error) {
		calls++
		if calls == 1 {
			return 0, errors.New("transient")
		}
		return 1, nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("Retry() = %v after %d calls", err, calls)
	}
}
