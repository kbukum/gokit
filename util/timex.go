package util

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// FormatDuration renders d as a human-readable string, choosing the largest unit for
// which the value reads naturally: hours and minutes with two decimals, seconds with
// two decimals down to one second, then integer milliseconds, microseconds ("μs"),
// or nanoseconds.
//
//	FormatDuration(5 * time.Second) == "5.00s"
//	FormatDuration(152 * time.Millisecond) == "152ms"
func FormatDuration(d time.Duration) string {
	secs := d.Seconds()
	switch {
	case secs >= 3600:
		return fmt.Sprintf("%.2fh", secs/3600)
	case secs >= 60:
		return fmt.Sprintf("%.2fm", secs/60)
	case secs >= 1:
		return fmt.Sprintf("%.2fs", secs)
	case d.Microseconds() >= 1000:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d.Nanoseconds() >= 1000:
		return fmt.Sprintf("%dμs", d.Microseconds())
	default:
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
}

// FormatDurationExact renders d as a lossless, round-trip-safe string, choosing the largest
// time unit that represents the duration as an exact integer so ParseDuration reconstructs the
// original value. Unlike FormatDuration, which rounds to two decimals for display, it never
// rounds. Use it for any duration serialized to configuration or a cross-kit wire contract.
//
//	FormatDurationExact(3601 * time.Second) == "3601s"
//	FormatDurationExact(2 * time.Hour) == "2h"
//	FormatDurationExact(1500 * time.Millisecond) == "1500ms"
func FormatDurationExact(d time.Duration) string {
	nanos := d.Nanoseconds()
	if nanos == 0 {
		return "0s"
	}
	switch {
	case nanos%int64(time.Hour) == 0:
		return fmt.Sprintf("%dh", nanos/int64(time.Hour))
	case nanos%int64(time.Minute) == 0:
		return fmt.Sprintf("%dm", nanos/int64(time.Minute))
	case nanos%int64(time.Second) == 0:
		return fmt.Sprintf("%ds", nanos/int64(time.Second))
	case nanos%int64(time.Millisecond) == 0:
		return fmt.Sprintf("%dms", nanos/int64(time.Millisecond))
	case nanos%int64(time.Microsecond) == 0:
		return fmt.Sprintf("%dus", nanos/int64(time.Microsecond))
	default:
		return fmt.Sprintf("%dns", nanos)
	}
}

// ParseDuration parses a duration string such as "5s", "10m", or "1.5h" into a
// time.Duration. Parsing is case-insensitive, allows optional whitespace before the
// unit, and treats a unit-less value as seconds. Recognized units are ns, us/μs, ms,
// s, m/min, h/hr, and d/day. It returns (0, false) on an unknown unit, a negative or
// malformed magnitude, or overflow.
//
//	ParseDuration("5") == (5*time.Second, true)
//	ParseDuration("10m") == (10*time.Minute, true)
func ParseDuration(s string) (time.Duration, bool) {
	normalized := strings.ToLower(strings.TrimSpace(s))
	splitIdx := strings.IndexFunc(normalized, unicode.IsLetter)
	if splitIdx < 0 {
		splitIdx = len(normalized)
	}
	numberPart := strings.TrimSpace(normalized[:splitIdx])
	unit := strings.TrimSpace(normalized[splitIdx:])

	var nanosPerUnit uint64
	switch unit {
	case "ns":
		nanosPerUnit = 1
	case "us", "μs":
		nanosPerUnit = 1_000
	case "ms":
		nanosPerUnit = 1_000_000
	case "s", "":
		nanosPerUnit = 1_000_000_000
	case "m", "min":
		nanosPerUnit = 60 * 1_000_000_000
	case "h", "hr":
		nanosPerUnit = 3_600 * 1_000_000_000
	case "d", "day":
		nanosPerUnit = 86_400 * 1_000_000_000
	default:
		return 0, false
	}

	nanos, ok := parseDecimalScaled(numberPart, nanosPerUnit)
	if !ok || nanos > uint64(1<<63-1) {
		return 0, false
	}
	return time.Duration(nanos), true
}

// TimeIt runs fn and returns its result alongside the wall-clock time it took.
func TimeIt[T any](fn func() T) (T, time.Duration) {
	start := time.Now()
	result := fn()
	return result, time.Since(start)
}

// ElapsedMillis returns the non-negative elapsed milliseconds between two monotonic
// millisecond readings, saturating at zero when end precedes start.
func ElapsedMillis(start, end uint64) uint64 {
	if end < start {
		return 0
	}
	return end - start
}
