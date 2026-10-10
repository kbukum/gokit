package lease

import (
	"context"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// MaxLimit bounds the number of concurrent leases, and therefore the size of one renewal batch.
const MaxLimit = 1 << 16

// Config sets the renewal contract of a [Set].
type Config[K comparable] struct {
	// Checker confirms or denies keys in batches.
	Checker Checker[K]

	// Lease is how long one confirmation, or the admitting check, keeps a lease alive, measured from the check start.
	Lease time.Duration

	// Interval is the renewal cadence, measured between batch starts. It must be shorter than Lease.
	Interval time.Duration

	// Limit is the maximum number of concurrent leases, 1..MaxLimit.
	Limit int
	// Clock schedules elapsed-time work; nil uses the monotonic runtime clock.
	Clock util.TimerClock
	// ReportError receives checker failures. It must honor ctx and must not block; it never runs on the expiry worker.
	ReportError func(context.Context, error)
}

func (c *Config[K]) validate() error {
	switch {
	case util.IsNil(c.Checker):
		return apperrors.InvalidInput("lease.checker", "checker is required")
	case c.ReportError == nil:
		return apperrors.InvalidInput("lease.report_error", "error reporter is required")
	case c.Clock != nil && util.IsNil(c.Clock):
		return apperrors.InvalidInput("lease.clock", "clock cannot be typed nil")
	case c.Lease <= 0:
		return apperrors.InvalidInput("lease.lease", "lease must be positive")
	case c.Interval <= 0 || c.Interval >= c.Lease:
		return apperrors.InvalidInput("lease.interval", "interval must be positive and shorter than the lease")
	case c.Limit < 1 || c.Limit > MaxLimit:
		return apperrors.InvalidInput("lease.limit", "limit must be between 1 and 65536")
	}
	return nil
}
