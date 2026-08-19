package resilience

import (
	"testing"
	"time"
)

func TestCalculateBackoffBounds(t *testing.T) {
	t.Parallel()
	minDelay := 100 * time.Millisecond
	maxDelay := 5 * time.Second
	for attempt := 1; attempt < 10; attempt++ {
		got := CalculateBackoff(attempt, minDelay, maxDelay)
		if got < minDelay || got > maxDelay {
			t.Errorf("attempt %d: %v out of [%v,%v]", attempt, got, minDelay, maxDelay)
		}
	}
}

func TestCalculateBackoffFirstAttempt(t *testing.T) {
	t.Parallel()
	minDelay := 100 * time.Millisecond
	if got := CalculateBackoff(1, minDelay, 5*time.Second); got != minDelay {
		t.Errorf("attempt 1 = %v, want %v", got, minDelay)
	}
}

func TestCalculateBackoffDoubles(t *testing.T) {
	t.Parallel()
	minDelay := 100 * time.Millisecond
	maxDelay := 10 * time.Second
	if got := CalculateBackoff(2, minDelay, maxDelay); got != 200*time.Millisecond {
		t.Errorf("attempt 2 = %v, want 200ms", got)
	}
	if got := CalculateBackoff(3, minDelay, maxDelay); got != 400*time.Millisecond {
		t.Errorf("attempt 3 = %v, want 400ms", got)
	}
}

func TestCalculateBackoffReachesCap(t *testing.T) {
	t.Parallel()
	minDelay := time.Nanosecond
	maxDelay := 5 * time.Second
	if got := CalculateBackoff(64, minDelay, maxDelay); got != maxDelay {
		t.Errorf("high attempt = %v, want cap %v", got, maxDelay)
	}
}

func TestCalculateJitteredBackoff(t *testing.T) {
	t.Parallel()
	minDelay := 100 * time.Millisecond
	maxDelay := 5 * time.Second

	zero, ok := CalculateJitteredBackoff(3, minDelay, maxDelay, 0)
	if !ok || zero != minDelay {
		t.Errorf("jitter 0 = (%v, %v), want min", zero, ok)
	}

	full, ok := CalculateJitteredBackoff(3, minDelay, maxDelay, jitterScale)
	if !ok || full != CalculateBackoff(3, minDelay, maxDelay) {
		t.Errorf("jitter max = (%v, %v), want capped backoff", full, ok)
	}

	if _, ok := CalculateJitteredBackoff(3, minDelay, maxDelay, jitterScale+1); ok {
		t.Error("out-of-range ratio should report false")
	}
}

func TestBackoffCalculator(t *testing.T) {
	t.Parallel()
	calc := NewBackoffCalculator(100*time.Millisecond, 5*time.Second)
	if calc.Delay(1) != 100*time.Millisecond {
		t.Errorf("Delay(1) = %v", calc.Delay(1))
	}
	if got, ok := calc.JitteredDelay(3, 0); !ok || got != 100*time.Millisecond {
		t.Errorf("JitteredDelay = (%v, %v)", got, ok)
	}
}
