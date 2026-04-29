package util

import (
	"testing"
	"time"
)

func TestSystemClockReturnsUTC(t *testing.T) {
	c := SystemClock{}
	now := c.Now()
	diff := time.Since(now)
	if diff < 0 || diff > 2*time.Second {
		t.Errorf("SystemClock.Now() off by %v", diff)
	}
	if now.Location() != time.UTC {
		t.Error("SystemClock.Now() should return UTC")
	}
}

func TestFakeClockDefault(t *testing.T) {
	c := NewFakeClock(time.Time{})
	expected := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if !c.Now().Equal(expected) {
		t.Errorf("default = %v, want %v", c.Now(), expected)
	}
}

func TestFakeClockCustomInitial(t *testing.T) {
	initial := time.Date(2020, 5, 1, 8, 30, 0, 0, time.UTC)
	c := NewFakeClock(initial)
	if !c.Now().Equal(initial) {
		t.Errorf("got %v, want %v", c.Now(), initial)
	}
}

func TestFakeClockAdvance(t *testing.T) {
	c := NewFakeClock(time.Time{})
	c.Advance(30 * time.Second)
	expected := time.Date(2024, 1, 1, 0, 0, 30, 0, time.UTC)
	if !c.Now().Equal(expected) {
		t.Errorf("got %v, want %v", c.Now(), expected)
	}
}

func TestFakeClockSet(t *testing.T) {
	c := NewFakeClock(time.Time{})
	target := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	c.Set(target)
	if !c.Now().Equal(target) {
		t.Errorf("got %v, want %v", c.Now(), target)
	}
}
