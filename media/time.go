package media

import (
	"fmt"
	"math"
	"time"
)

// Timestamp is a time point in microseconds from the start of the media.
//
// Microsecond precision matches the internal timestamp resolution of common
// container and codec formats, avoiding precision loss during conversion. A
// Timestamp is never negative: constructors and arithmetic clamp at zero and
// saturate at the maximum representable value on overflow.
//
// It is the light-kit parallel of rskit's media Timestamp.
type Timestamp int64

// TimestampFromMillis builds a [Timestamp] from a millisecond value.
func TimestampFromMillis(ms int64) Timestamp {
	if ms <= 0 {
		return 0
	}
	if ms > math.MaxInt64/1000 {
		return math.MaxInt64
	}
	return Timestamp(ms * 1000)
}

// TimestampFromMicros builds a [Timestamp] from a microsecond value.
func TimestampFromMicros(us int64) Timestamp {
	if us < 0 {
		us = 0
	}
	return Timestamp(us)
}

// TimestampFromSeconds builds a [Timestamp] from a floating-point seconds value.
func TimestampFromSeconds(s float64) Timestamp {
	us := s * 1_000_000
	if !(us > 0) { // false for negatives, zero, and NaN
		return 0
	}
	if us >= math.MaxInt64 {
		return math.MaxInt64
	}
	return Timestamp(us)
}

// TimestampFromDuration builds a [Timestamp] from a [time.Duration].
func TimestampFromDuration(d time.Duration) Timestamp {
	if d < 0 {
		d = 0
	}
	return Timestamp(d.Microseconds())
}

// Millis returns the value in whole milliseconds, truncating sub-millisecond
// precision.
func (t Timestamp) Millis() int64 { return int64(t) / 1000 }

// Micros returns the value in microseconds.
func (t Timestamp) Micros() int64 { return int64(t) }

// Seconds returns the value as floating-point seconds.
func (t Timestamp) Seconds() float64 { return float64(t) / 1_000_000 }

// Duration returns the value as a [time.Duration].
func (t Timestamp) Duration() time.Duration { return time.Duration(t) * time.Microsecond }

// Add shifts the timestamp by a signed offset, clamping the result at zero and
// saturating at the maximum representable value on overflow.
func (t Timestamp) Add(offset time.Duration) Timestamp {
	cur := int64(t)
	off := offset.Microseconds()
	us := cur + off
	if off > 0 && us < cur { // positive overflow wrapped around
		return math.MaxInt64
	}
	if us < 0 {
		return 0
	}
	return Timestamp(us)
}

// String formats the timestamp as "HH:MM:SS.mmm".
func (t Timestamp) String() string {
	us := int64(t)
	ms := (us / 1000) % 1000
	totalSecs := us / 1_000_000
	secs := totalSecs % 60
	totalMins := totalSecs / 60
	mins := totalMins % 60
	hours := totalMins / 60
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, mins, secs, ms)
}

// TimeRange is a half-open time window [Start, End) within a media file: Start
// is included, End is excluded. Overlaps, Contains, Split, and Merge all follow
// this model, so touching endpoints do not overlap.
type TimeRange struct {
	Start Timestamp `json:"start"`
	End   Timestamp `json:"end"`
}

// NewTimeRange builds a [TimeRange] from two timestamps.
func NewTimeRange(start, end Timestamp) TimeRange {
	return TimeRange{Start: start, End: end}
}

// TimeRangeFromMillis builds a [TimeRange] from millisecond bounds.
func TimeRangeFromMillis(startMs, endMs int64) TimeRange {
	return TimeRange{Start: TimestampFromMillis(startMs), End: TimestampFromMillis(endMs)}
}

// Duration returns the length of the range, or zero when End precedes Start.
func (r TimeRange) Duration() time.Duration {
	if r.End <= r.Start {
		return 0
	}
	return (r.End - r.Start).Duration()
}

// Contains reports whether ts falls within the half-open range [Start, End).
func (r TimeRange) Contains(ts Timestamp) bool {
	return ts >= r.Start && ts < r.End
}

// Overlaps reports whether this range and other share any instant.
func (r TimeRange) Overlaps(other TimeRange) bool {
	return r.Start < other.End && other.Start < r.End
}

// Merge combines two overlapping ranges into their union. The bool is false
// when the ranges do not overlap, in which case the returned range is unset.
func (r TimeRange) Merge(other TimeRange) (TimeRange, bool) {
	if !r.Overlaps(other) {
		return TimeRange{}, false
	}
	return TimeRange{Start: min(r.Start, other.Start), End: max(r.End, other.End)}, true
}

// Split divides the range at ts. It returns the two resulting sub-ranges when
// ts lies strictly inside the range; otherwise it returns the range unchanged
// as a single element.
func (r TimeRange) Split(ts Timestamp) []TimeRange {
	if ts <= r.Start || ts >= r.End {
		return []TimeRange{r}
	}
	return []TimeRange{{Start: r.Start, End: ts}, {Start: ts, End: r.End}}
}

// Shift moves both bounds by a signed offset, clamping each at zero.
func (r TimeRange) Shift(offset time.Duration) TimeRange {
	return TimeRange{Start: r.Start.Add(offset), End: r.End.Add(offset)}
}

// Segment is a labeled time range, useful for chapters, scenes, or detected
// spans. It is the light-kit parallel of rskit's media Segment, without the
// backend-specific metadata bag.
type Segment struct {
	Range TimeRange `json:"range"`
	// Label is an optional human-readable name (e.g. "intro", "chorus").
	Label string `json:"label,omitempty"`
	// Confidence is an optional score in the range [0, 1]; zero means unset.
	Confidence float64 `json:"confidence,omitempty"`
}

// NewSegment builds a [Segment] covering the given range.
func NewSegment(r TimeRange) Segment { return Segment{Range: r} }

// WithLabel returns a copy of the segment with the given label.
func (s Segment) WithLabel(label string) Segment {
	s.Label = label
	return s
}

// WithConfidence returns a copy of the segment with the confidence clamped to
// the range [0, 1].
func (s Segment) WithConfidence(c float64) Segment {
	s.Confidence = min(max(c, 0), 1)
	return s
}
