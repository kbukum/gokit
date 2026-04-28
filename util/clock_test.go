package util

import (
	"testing"
	"time"
)

func TestSystemClock_Now(t *testing.T) {
	c := SystemClock{}
	now := c.Now()
	diff := time.Since(now)
	if diff < 0 || diff > 2*time.Second {
		t.Errorf("SystemClock.Now() returned %v, expected close to now", now)
	}
}

func TestFakeClock_Default(t *testing.T) {
	c := NewFakeClock(time.Time{})
	expected := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := c.Now(); !got.Equal(expected) {
		t.Errorf("default FakeClock = %v, want %v", got, expected)
	}
}

func TestFakeClock_Advance(t *testing.T) {
	c := NewFakeClock(time.Time{})
	c.Advance(30 * time.Second)
	expected := time.Date(2024, 1, 1, 0, 0, 30, 0, time.UTC)
	if got := c.Now(); !got.Equal(expected) {
		t.Errorf("after advance = %v, want %v", got, expected)
	}
}

func TestFakeClock_Set(t *testing.T) {
	c := NewFakeClock(time.Time{})
	target := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	c.Set(target)
	if got := c.Now(); !got.Equal(target) {
		t.Errorf("after set = %v, want %v", got, target)
	}
}

func TestFakeClock_CustomInitial(t *testing.T) {
	initial := time.Date(2020, 5, 1, 8, 30, 0, 0, time.UTC)
	c := NewFakeClock(initial)
	if got := c.Now(); !got.Equal(initial) {
		t.Errorf("custom initial = %v, want %v", got, initial)
	}
}
