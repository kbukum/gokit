package resilience

import "time"

// RetryPreset names a retry configuration tuned for a common integration pattern.
// Presets keep call sites declarative and consistent with the sibling kits.
type RetryPreset int

const (
	// RetryFast is a short retry loop for local tests and latency-sensitive operations.
	RetryFast RetryPreset = iota
	// RetryStandard is the balanced default for general service-to-service calls.
	RetryStandard
	// RetryExternalService is a more tolerant policy for external network dependencies.
	RetryExternalService
)

// Config returns the RetryConfig represented by the preset.
func (p RetryPreset) Config() RetryConfig {
	switch p {
	case RetryFast:
		return RetryConfig{
			MaxAttempts:    2,
			Strategy:       ConstantBackoff,
			InitialBackoff: 10 * time.Millisecond,
			MaxBackoff:     10 * time.Millisecond,
			MaxElapsedTime: 1 * time.Second,
			Jitter:         0.1,
			RetryIf:        DefaultRetryIf,
		}
	case RetryExternalService:
		return RetryConfig{
			MaxAttempts:    4,
			Strategy:       ExponentialBackoff,
			InitialBackoff: 200 * time.Millisecond,
			MaxBackoff:     5 * time.Second,
			BackoffFactor:  2.0,
			MaxElapsedTime: 30 * time.Second,
			Jitter:         0.1,
			RetryIf:        DefaultRetryIf,
		}
	default:
		return RetryConfig{
			MaxAttempts:    3,
			Strategy:       ExponentialBackoff,
			InitialBackoff: 100 * time.Millisecond,
			MaxBackoff:     2 * time.Second,
			BackoffFactor:  2.0,
			MaxElapsedTime: 10 * time.Second,
			Jitter:         0.1,
			RetryIf:        DefaultRetryIf,
		}
	}
}
