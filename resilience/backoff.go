package resilience

import "time"

// CalculateBackoff computes a capped exponential backoff delay for a one-based
// attempt index. It is fully stateless — it never sleeps or schedules timers — so it
// suits callers that only need the delay value (retry-after math, transport helpers)
// rather than the full RetryConfig-driven Retry loop. The first attempt returns
// minDelay; each subsequent attempt doubles the previous delay, clamped to maxDelay.
// A non-positive minDelay, or a maxDelay at or below minDelay, returns minDelay.
func CalculateBackoff(attempt int, minDelay, maxDelay time.Duration) time.Duration {
	if maxDelay <= minDelay || attempt <= 1 || minDelay <= 0 {
		return minDelay
	}
	delay := minDelay
	for i := 1; i < attempt; i++ {
		if delay > maxDelay/2 {
			return maxDelay
		}
		next := delay * 2
		if next >= maxDelay {
			return maxDelay
		}
		delay = next
	}
	return delay
}

// jitterScale is the fixed-point denominator for CalculateJitteredBackoff's ratio.
const jitterScale = 10_000

// CalculateJitteredBackoff computes a deterministic jittered delay inside
// [minDelay, CalculateBackoff(...)]. jitterPermyriad is an integer ratio in the
// inclusive range [0, 10000], where 0 returns minDelay and 10000 returns the full
// capped exponential delay; the delay scales linearly in between. This keeps jitter
// injectable and reproducible under test rather than reaching for a global RNG. It
// returns (0, false) when jitterPermyriad exceeds 10000.
func CalculateJitteredBackoff(attempt int, minDelay, maxDelay time.Duration, jitterPermyriad int) (time.Duration, bool) {
	if jitterPermyriad < 0 || jitterPermyriad > jitterScale {
		return 0, false
	}
	limit := CalculateBackoff(attempt, minDelay, maxDelay)
	if limit <= minDelay {
		return minDelay, true
	}
	span := int64(limit - minDelay)
	jitter := span / jitterScale * int64(jitterPermyriad)
	remainder := span % jitterScale * int64(jitterPermyriad) / jitterScale
	return minDelay + time.Duration(jitter+remainder), true
}

// BackoffCalculator is a reusable, immutable exponential-backoff configuration over
// CalculateBackoff and CalculateJitteredBackoff. It is the stateless-math counterpart
// to the RetryConfig-driven Retry loop; the type is named BackoffCalculator to avoid
// colliding with ExponentialBackoff, which is already the
// name of a BackoffStrategy constant in this package.
type BackoffCalculator struct {
	minDelay time.Duration
	maxDelay time.Duration
}

// NewBackoffCalculator creates a calculator bounded by minDelay and maxDelay.
func NewBackoffCalculator(minDelay, maxDelay time.Duration) BackoffCalculator {
	return BackoffCalculator{minDelay: minDelay, maxDelay: maxDelay}
}

// Delay returns the capped exponential delay for a one-based attempt index.
func (b BackoffCalculator) Delay(attempt int) time.Duration {
	return CalculateBackoff(attempt, b.minDelay, b.maxDelay)
}

// JitteredDelay returns the jittered delay for an attempt using an injected ratio.
func (b BackoffCalculator) JitteredDelay(attempt, jitterPermyriad int) (time.Duration, bool) {
	return CalculateJitteredBackoff(attempt, b.minDelay, b.maxDelay, jitterPermyriad)
}
