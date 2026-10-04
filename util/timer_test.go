package util

import (
	"testing"
	"testing/synctest"
	"time"
)

func TestFakeClockTimers(t *testing.T) {
	c := NewFakeClock(time.Time{})
	timer := c.NewTimer(time.Second)
	c.Advance(999 * time.Millisecond)
	select {
	case <-timer.C():
		t.Fatal("early timer")
	default:
	}
	c.Advance(time.Millisecond)
	select {
	case <-timer.C():
	default:
		t.Fatal("missing timer")
	}
	if timer.Stop() {
		t.Fatal("expired stop")
	}
	timer = c.NewTimer(time.Second)
	if !timer.Stop() {
		t.Fatal("live stop")
	}
	c.Advance(time.Second)
	select {
	case <-timer.C():
		t.Fatal("stopped timer")
	default:
	}
}

func TestMonotonicClockTimer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := MonotonicClock{}
		start := clock.Now()
		timer := clock.NewTimer(2 * time.Second)
		defer timer.Stop()
		time.Sleep(2 * time.Second)
		synctest.Wait()
		select {
		case <-timer.C():
		default:
			t.Fatal("runtime timer did not advance")
		}
		if clock.Now().Sub(start) != 2*time.Second {
			t.Fatal("elapsed monotonic time")
		}
	})
}
