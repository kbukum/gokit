//go:build !windows

package process

import (
	"errors"
	"syscall"
	"testing"
	"time"
)

func TestStartRetryingETXTBSYRetriesTransientBusy(t *testing.T) {
	t.Parallel()
	attempts := 0
	err := startRetryingETXTBSY(func() error {
		attempts++
		if attempts < 3 {
			return syscall.ETXTBSY
		}
		return nil
	}, func(time.Duration) {})
	if err != nil {
		t.Fatalf("expected success once the busy window closes, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected the start to be retried past the busy window, got %d attempts", attempts)
	}
}

func TestStartRetryingETXTBSYDoesNotRetryOtherErrors(t *testing.T) {
	t.Parallel()
	attempts := 0
	err := startRetryingETXTBSY(func() error {
		attempts++
		return syscall.ENOENT
	}, func(time.Duration) { t.Fatal("must not back off for a non-ETXTBSY failure") })
	if !errors.Is(err, syscall.ENOENT) {
		t.Fatalf("expected a missing-binary error to surface immediately, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected a single attempt for a non-transient error, got %d", attempts)
	}
}

func TestStartRetryingETXTBSYBoundsAttempts(t *testing.T) {
	t.Parallel()
	attempts := 0
	err := startRetryingETXTBSY(func() error {
		attempts++
		return syscall.ETXTBSY
	}, func(time.Duration) {})
	if !errors.Is(err, syscall.ETXTBSY) {
		t.Fatalf("expected the final ETXTBSY to surface after exhausting retries, got %v", err)
	}
	if attempts != 10 {
		t.Fatalf("expected retries bounded to 10 attempts, got %d", attempts)
	}
}
