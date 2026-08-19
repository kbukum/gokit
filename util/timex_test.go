package util

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   time.Duration
		want string
	}{
		{2 * time.Hour, "2.00h"},
		{150 * time.Second, "2.50m"},
		{5 * time.Second, "5.00s"},
		{250 * time.Millisecond, "250ms"},
		{500 * time.Microsecond, "500μs"},
		{42 * time.Nanosecond, "42ns"},
	}
	for _, c := range cases {
		if got := FormatDuration(c.in); got != c.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"5", 5 * time.Second},
		{"5s", 5 * time.Second},
		{"10m", 10 * time.Minute},
		{"1.5h", 90 * time.Minute},
		{"1.5ms", 1500 * time.Microsecond},
		{"1d", 24 * time.Hour},
	}
	for _, c := range cases {
		got, ok := ParseDuration(c.in)
		if !ok || got != c.want {
			t.Errorf("ParseDuration(%q) = (%v, %v), want %v", c.in, got, ok, c.want)
		}
	}
}

func TestParseDurationInvalid(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"-1s", "invalid", "5xy"} {
		if _, ok := ParseDuration(in); ok {
			t.Errorf("ParseDuration(%q) should fail", in)
		}
	}
}

func TestTimeIt(t *testing.T) {
	t.Parallel()
	result, elapsed := TimeIt(func() int { return 42 })
	if result != 42 {
		t.Errorf("result = %d", result)
	}
	if elapsed < 0 {
		t.Errorf("elapsed negative: %v", elapsed)
	}
}

func TestElapsedMillis(t *testing.T) {
	t.Parallel()
	if got := ElapsedMillis(10, 25); got != 15 {
		t.Errorf("ElapsedMillis(10,25) = %d", got)
	}
	if got := ElapsedMillis(25, 10); got != 0 {
		t.Errorf("ElapsedMillis(25,10) = %d, want 0", got)
	}
}
