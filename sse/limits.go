package sse

import (
	"strings"
	"unicode/utf8"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

const MaxRoutingBytes = 512

// Limits bounds stored replay, per-connection live queues, frames, and connection admission. ReplayBytes includes encoded frames and routing patterns.
type Limits struct {
	ReplayEvents    int
	ReplayBytes     int
	QueueEvents     int
	MaxEventBytes   int
	MaxConnections  int
	MaxPerPrincipal int
}

// DefaultLimits returns independent value-owned limits for a single instance.
func DefaultLimits() Limits {
	return Limits{ReplayEvents: 1024, ReplayBytes: 8 << 20, QueueEvents: 32, MaxEventBytes: 64 << 10, MaxConnections: 1024, MaxPerPrincipal: 8}
}

func (l Limits) validate() error {
	if l.ReplayEvents < 1 || l.ReplayEvents > 1<<20 || l.ReplayBytes < 256 || l.ReplayBytes > 1<<30 ||
		l.QueueEvents < 1 || l.QueueEvents > 1<<16 || l.MaxEventBytes < 256 || l.MaxEventBytes > 1<<20 ||
		l.MaxConnections < 1 || l.MaxConnections > 1<<20 || l.MaxPerPrincipal < 1 || l.MaxPerPrincipal > l.MaxConnections {
		return apperrors.InvalidInput("limits", "SSE limits must be positive and within documented bounds")
	}
	return nil
}

func validateKey(key string, pattern bool) error {
	if key == "" || len(key) > MaxRoutingBytes || !utf8.ValidString(key) ||
		strings.ContainsFunc(key, func(r rune) bool { return r < 32 || r == 127 }) || (!pattern && util.HasWildcard(key)) {
		return apperrors.InvalidInput("scope", "SSE scope must be a bounded nonempty routing key")
	}
	return nil
}

// Stats is an atomic bus snapshot suitable for collection by an injected metrics adapter. Counters are lifetime totals; gauges include admitted terminal streams until Close.
type Stats struct {
	ActiveStreams       int
	QueueDepth          int
	QueueBytes          int
	ReplayEvents        int
	ReplayBytes         int
	Drops               uint64
	Resets              uint64
	RejectedConnections uint64
	AllocatedQueues     uint64
}

// Stats reads resource gauges and monotonic counters without principal or route labels.
func (b *Bus) Stats() Stats {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.stats
	s.ActiveStreams, s.ReplayEvents, s.ReplayBytes = len(b.subs), b.count, b.replaySize
	return s
}
