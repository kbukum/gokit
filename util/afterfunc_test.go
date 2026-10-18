package util

import (
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestAfterFunc(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)

	t.Run("stop before expiry prevents fn", func(t *testing.T) {
		clock := NewFakeClock(epoch)
		var ran atomic.Bool
		stop := AfterFunc(clock, time.Minute, func() { ran.Store(true) })
		if !stop() {
			t.Fatal("stop before expiry reported fn ran")
		}
		clock.Advance(time.Hour)
		if ran.Load() {
			t.Fatal("fn ran after stop")
		}
	})

	t.Run("expiry runs fn once", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			clock := NewFakeClock(epoch)
			var calls atomic.Int32
			stop := AfterFunc(clock, time.Minute, func() { calls.Add(1) })
			clock.Advance(time.Minute)
			synctest.Wait()
			if stop() {
				t.Fatal("stop after expiry reported fn prevented")
			}
			if got := calls.Load(); got != 1 {
				t.Fatalf("fn ran %d times", got)
			}
		})
	})

	t.Run("stop waits for running fn", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			clock := NewFakeClock(epoch)
			release := make(chan struct{})
			stop := AfterFunc(clock, time.Minute, func() { <-release })
			clock.Advance(time.Minute)
			synctest.Wait()
			var stopped atomic.Bool
			prevented := make(chan bool, 1)
			go func() {
				p := stop()
				stopped.Store(true)
				prevented <- p
			}()
			synctest.Wait()
			if stopped.Load() {
				t.Fatal("stop returned while fn was running")
			}
			close(release)
			if <-prevented {
				t.Fatal("stop reported a running fn prevented")
			}
		})
	})

	t.Run("non-positive duration is due at once", func(t *testing.T) {
		for _, d := range []time.Duration{0, -time.Second} {
			synctest.Test(t, func(t *testing.T) {
				var ran atomic.Bool
				stop := AfterFunc(NewFakeClock(epoch), d, func() { ran.Store(true) })
				synctest.Wait()
				if stop() || !ran.Load() {
					t.Fatalf("d=%s: fn did not run at once", d)
				}
			})
		}
	})

	t.Run("fn runs exactly when stop reports it was not prevented", func(t *testing.T) {
		for range 500 {
			clock := NewFakeClock(epoch)
			var ran atomic.Bool
			stop := AfterFunc(clock, time.Millisecond, func() { ran.Store(true) })
			go clock.Advance(time.Millisecond)
			prevented := stop()
			if prevented == ran.Load() {
				t.Fatalf("stop reported prevented=%t but fn ran=%t", prevented, ran.Load())
			}
			clock.Advance(time.Millisecond)
			if prevented && ran.Load() {
				t.Fatal("fn ran after stop")
			}
		}
	})
}
