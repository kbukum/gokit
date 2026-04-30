package worker

import (
	"encoding"
	"net/http"

	gkerrors "github.com/kbukum/gokit/errors"
)

var (
	_ encoding.TextMarshaler   = OverflowPolicy("")
	_ encoding.TextUnmarshaler = (*OverflowPolicy)(nil)
)

// OverflowPolicy controls what happens when the pool queue is full.
type OverflowPolicy string

const (
	// OverflowBlock waits until queue capacity is available.
	OverflowBlock OverflowPolicy = "block"
	// OverflowReject fails the submission immediately.
	OverflowReject OverflowPolicy = "reject"
	// OverflowDropOldest evicts the oldest queued task to make room.
	OverflowDropOldest OverflowPolicy = "drop_oldest"
)

var (
	// ErrQueueFull is returned when a task cannot be enqueued immediately.
	ErrQueueFull = gkerrors.New(gkerrors.ErrCodeRateLimited, "worker queue is full", http.StatusTooManyRequests)
	// ErrTaskDropped is reported to a task that was evicted by DropOldest.
	ErrTaskDropped = gkerrors.Canceled("worker task dropped due to overflow")
)

// MarshalText serializes an overflow policy for config encoders.
func (o OverflowPolicy) MarshalText() ([]byte, error) {
	policy := o
	if policy == "" {
		policy = OverflowBlock
	}
	if !policy.valid() {
		return nil, invalidOverflowPolicy(policy)
	}
	return []byte(policy), nil
}

// UnmarshalText parses an overflow policy from config text.
func (o *OverflowPolicy) UnmarshalText(text []byte) error {
	policy := OverflowPolicy(text)
	if policy == "" {
		*o = OverflowBlock
		return nil
	}
	if !policy.valid() {
		return invalidOverflowPolicy(policy)
	}
	*o = policy
	return nil
}

func (o OverflowPolicy) valid() bool {
	switch o {
	case OverflowBlock, OverflowReject, OverflowDropOldest:
		return true
	default:
		return false
	}
}

func invalidOverflowPolicy(policy OverflowPolicy) error {
	return gkerrors.InvalidInput("overflow", "must be one of block, reject, drop_oldest").WithDetail("value", string(policy))
}
