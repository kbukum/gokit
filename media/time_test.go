package media

import (
	"testing"
	"time"
)

func TestTimestamp_Constructors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		got  Timestamp
		want Timestamp
	}{
		{"millis", TimestampFromMillis(1500), 1_500_000},
		{"millis negative clamps", TimestampFromMillis(-5), 0},
		{"micros", TimestampFromMicros(42), 42},
		{"micros negative clamps", TimestampFromMicros(-1), 0},
		{"seconds", TimestampFromSeconds(2.5), 2_500_000},
		{"seconds negative clamps", TimestampFromSeconds(-2), 0},
		{"duration", TimestampFromDuration(3 * time.Second), 3_000_000},
		{"duration negative clamps", TimestampFromDuration(-time.Second), 0},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

func TestTimestamp_Accessors(t *testing.T) {
	t.Parallel()
	ts := TimestampFromMicros(2_500_500)
	if ts.Millis() != 2500 {
		t.Errorf("Millis = %d, want 2500", ts.Millis())
	}
	if ts.Micros() != 2_500_500 {
		t.Errorf("Micros = %d, want 2500500", ts.Micros())
	}
	if ts.Seconds() != 2.5005 {
		t.Errorf("Seconds = %v, want 2.5005", ts.Seconds())
	}
	if ts.Duration() != 2_500_500*time.Microsecond {
		t.Errorf("Duration = %v", ts.Duration())
	}
}

func TestTimestamp_Add(t *testing.T) {
	t.Parallel()
	ts := TimestampFromMillis(1000)
	if got := ts.Add(500 * time.Millisecond); got != TimestampFromMillis(1500) {
		t.Errorf("Add positive = %d", got)
	}
	if got := ts.Add(-2 * time.Second); got != 0 {
		t.Errorf("Add past zero should clamp, got %d", got)
	}
}

func TestTimestamp_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ts   Timestamp
		want string
	}{
		{TimestampFromMillis(0), "00:00:00.000"},
		{TimestampFromMillis(1250), "00:00:01.250"},
		{TimestampFromMillis(3_661_007), "01:01:01.007"},
	}
	for _, tt := range tests {
		if got := tt.ts.String(); got != tt.want {
			t.Errorf("String(%d) = %q, want %q", tt.ts, got, tt.want)
		}
	}
}

func TestTimeRange_DurationAndContains(t *testing.T) {
	t.Parallel()
	r := TimeRangeFromMillis(1000, 3000)
	if r.Duration() != 2*time.Second {
		t.Errorf("Duration = %v, want 2s", r.Duration())
	}
	if got := NewTimeRange(TimestampFromMillis(5), TimestampFromMillis(1)); got.Duration() != 0 {
		t.Errorf("reversed range Duration = %v, want 0", got.Duration())
	}
	if !r.Contains(TimestampFromMillis(2000)) {
		t.Error("expected Contains(2000)")
	}
	if r.Contains(TimestampFromMillis(3001)) {
		t.Error("did not expect Contains(3001)")
	}
}

func TestTimeRange_Overlaps(t *testing.T) {
	t.Parallel()
	a := TimeRangeFromMillis(0, 1000)
	if !a.Overlaps(TimeRangeFromMillis(500, 1500)) {
		t.Error("expected overlap")
	}
	if a.Overlaps(TimeRangeFromMillis(1000, 2000)) {
		t.Error("touching endpoints should not overlap")
	}
}

func TestTimeRange_Merge(t *testing.T) {
	t.Parallel()
	a := TimeRangeFromMillis(0, 1000)
	merged, ok := a.Merge(TimeRangeFromMillis(500, 2000))
	if !ok || merged != TimeRangeFromMillis(0, 2000) {
		t.Errorf("Merge = %v ok=%v, want 0-2000", merged, ok)
	}
	if _, ok := a.Merge(TimeRangeFromMillis(2000, 3000)); ok {
		t.Error("non-overlapping Merge should report false")
	}
}

func TestTimeRange_Split(t *testing.T) {
	t.Parallel()
	r := TimeRangeFromMillis(0, 1000)
	parts := r.Split(TimestampFromMillis(400))
	if len(parts) != 2 || parts[0] != TimeRangeFromMillis(0, 400) || parts[1] != TimeRangeFromMillis(400, 1000) {
		t.Errorf("Split inside = %v", parts)
	}
	if got := r.Split(TimestampFromMillis(0)); len(got) != 1 || got[0] != r {
		t.Errorf("Split at start should be no-op, got %v", got)
	}
	if got := r.Split(TimestampFromMillis(1000)); len(got) != 1 || got[0] != r {
		t.Errorf("Split at end should be no-op, got %v", got)
	}
}

func TestTimeRange_Shift(t *testing.T) {
	t.Parallel()
	r := TimeRangeFromMillis(1000, 2000)
	if got := r.Shift(500 * time.Millisecond); got != TimeRangeFromMillis(1500, 2500) {
		t.Errorf("Shift = %v", got)
	}
	if got := r.Shift(-5 * time.Second); got.Start != 0 {
		t.Errorf("Shift past zero should clamp start, got %v", got)
	}
}

func TestSegment_Builders(t *testing.T) {
	t.Parallel()
	s := NewSegment(TimeRangeFromMillis(0, 1000)).WithLabel("intro").WithConfidence(0.7)
	if s.Label != "intro" || s.Confidence != 0.7 {
		t.Errorf("segment = %+v", s)
	}
	if got := s.WithConfidence(2.0); got.Confidence != 1 {
		t.Errorf("confidence should clamp high, got %v", got.Confidence)
	}
	if got := s.WithConfidence(-1.0); got.Confidence != 0 {
		t.Errorf("confidence should clamp low, got %v", got.Confidence)
	}
}
