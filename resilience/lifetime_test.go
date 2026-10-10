package resilience

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestAdmissionHoldsCapacityUntilFinished(t *testing.T) {
	t.Parallel()

	var acquired, released atomic.Int32
	p := NewPolicy().WithBulkhead(BulkheadConfig{
		MaxConcurrent: 2,
		OnAcquire:     func(string) { acquired.Add(1) },
		OnRelease:     func(string) { released.Add(1) },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, first, err := p.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer second(nil)
	cancel()
	if _, _, err := p.Acquire(context.Background()); !errors.Is(err, ErrBulkheadFull) {
		t.Fatalf("cancel released capacity before cleanup: %v", err)
	}
	if acquired.Load() != 2 || released.Load() != 0 {
		t.Fatalf("acquired=%d released=%d", acquired.Load(), released.Load())
	}
	if err := first(ctx.Err()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled admission returned success: %v", err)
	}
	_, next, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := next(nil); err != nil {
		t.Fatal(err)
	}
	if err := second(nil); err != nil {
		t.Fatal(err)
	}
	if acquired.Load() != 3 || released.Load() != 3 {
		t.Fatalf("acquired=%d released=%d; want 3/3", acquired.Load(), released.Load())
	}
}

func TestAdmissionConcurrentFinish(t *testing.T) {
	t.Parallel()

	var releases, retries atomic.Int32
	p := NewPolicy().
		WithBulkhead(BulkheadConfig{MaxConcurrent: 1, OnRelease: func(string) { releases.Add(1) }}).
		WithCircuitBreaker(CircuitBreakerConfig{MaxFailures: 2}).
		WithRetry(RetryConfig{MaxAttempts: 3, OnRetry: func(int, error, time.Duration) { retries.Add(1) }})
	ctx, finish, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("upstream failed")
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if err := finish(failure); !errors.Is(err, failure) {
				t.Errorf("terminal outcome changed: %v", err)
			}
		})
	}
	wg.Wait()
	if err := finish(nil); !errors.Is(err, failure) {
		t.Fatalf("second finish overwrote failure: %v", err)
	}
	if releases.Load() != 1 || p.cb.Failures() != 1 || retries.Load() != 0 {
		t.Fatalf("releases=%d failures=%d retries=%d; want 1/1/0", releases.Load(), p.cb.Failures(), retries.Load())
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("finish did not cancel the owned context")
	}
	if !p.IsAvailable() {
		t.Fatal("duplicate terminal reports opened the breaker")
	}
}

func TestAdmissionBudget(t *testing.T) {
	t.Parallel()

	for _, phase := range []string{"rate", "bulkhead", "active"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				p := NewPolicy().WithTimeout(10 * time.Millisecond).
					WithCircuitBreaker(CircuitBreakerConfig{MaxFailures: 1, Timeout: time.Second})
				switch phase {
				case "rate":
					p.WithRateLimiter(RateLimiterConfig{Rate: 1, Burst: 1})
					_, first, err := p.Acquire(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					if err := first(nil); err != nil {
						t.Fatal(err)
					}
				case "bulkhead":
					p.WithBulkhead(BulkheadConfig{MaxConcurrent: 1, MaxWait: time.Hour, MaxQueue: 1})
					_, first, err := p.Acquire(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					defer first(context.Canceled)
				}
				start := time.Now()
				ctx, finish, err := p.Acquire(context.Background())
				if phase == "active" {
					if err != nil {
						t.Fatal(err)
					}
					<-ctx.Done()
					err = finish(ctx.Err())
				} else if ctx != nil || finish != nil {
					t.Fatal("admitted work beyond its budget")
				}
				if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != 10*time.Millisecond {
					t.Fatalf("elapsed=%s outcome=%v; want deadline after 10ms", time.Since(start), err)
				}
				if p.IsAvailable() != (phase != "active") {
					t.Fatalf("breaker counted the wrong phase: %s", phase)
				}
			})
		})
	}
}

func TestAdmissionTimeoutModes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		existing time.Duration
		ifUnset  bool
		want     time.Duration
	}{
		{"policy only", 0, false, 10 * time.Millisecond},
		{"shorter caller", time.Millisecond, false, time.Millisecond},
		{"shorter policy", time.Second, false, 10 * time.Millisecond},
		{"preserve caller", time.Second, true, time.Second},
		{"unset caller", 0, true, 10 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				p := NewPolicy().WithTimeout(10 * time.Millisecond)
				if tc.ifUnset {
					p.WithTimeoutIfUnset(10 * time.Millisecond)
				}
				ctx := context.Background()
				if tc.existing != 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, tc.existing)
					defer cancel()
				}
				start := time.Now()
				callCtx, finish, err := p.Acquire(ctx)
				if err != nil {
					t.Fatal(err)
				}
				<-callCtx.Done()
				if err := finish(callCtx.Err()); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != tc.want {
					t.Fatalf("elapsed=%s outcome=%v; want %s", time.Since(start), err, tc.want)
				}
			})
		})
	}
}

func TestAdmissionCancellationAndRejection(t *testing.T) {
	t.Parallel()

	var acquisitions, releases atomic.Int32
	p := NewPolicy().
		WithBulkhead(BulkheadConfig{
			MaxConcurrent: 1,
			OnAcquire:     func(string) { acquisitions.Add(1) },
			OnRelease:     func(string) { releases.Add(1) },
		}).
		WithCircuitBreaker(CircuitBreakerConfig{MaxFailures: 2, Timeout: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if callCtx, finish, err := p.Acquire(ctx); callCtx != nil || finish != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller admitted: %v", err)
	}
	if acquisitions.Load() != 0 {
		t.Fatal("canceled caller acquired a slot")
	}
	_, finish, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("upstream failed")
	if err := finish(failure); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	_, finish, err = p.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := finish(fmt.Errorf("caller stopped: %w", context.Canceled)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if p.cb.Failures() != 1 {
		t.Fatalf("cancellation was not neutral: failures=%d", p.cb.Failures())
	}
	_, finish, err = p.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := finish(failure); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if ctx, finish, err := p.Acquire(context.Background()); ctx != nil || finish != nil || !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("open breaker admitted work: %v", err)
	}
	if acquisitions.Load() != releases.Load() || p.cb.Failures() != 2 {
		t.Fatalf("acquired=%d released=%d failures=%d", acquisitions.Load(), releases.Load(), p.cb.Failures())
	}
}

func TestAdmissionCancelledHalfOpenProbeCanBeReplaced(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		p := NewPolicy().WithCircuitBreaker(CircuitBreakerConfig{
			MaxFailures: 1, Timeout: time.Second, HalfOpenMaxCalls: 2,
		})
		_, finish, err := p.Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		failure := errors.New("trip")
		if err := finish(failure); !errors.Is(err, failure) {
			t.Fatal(err)
		}
		<-time.After(time.Second)
		_, first, err := p.Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		_, second, err := p.Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := p.Acquire(context.Background()); !errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("half-open quota exceeded: %v", err)
		}
		if err := first(context.Canceled); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		_, replacement, err := p.Acquire(context.Background())
		if err != nil {
			t.Fatalf("canceled probe retained capacity: %v", err)
		}
		if err := second(nil); err != nil {
			t.Fatal(err)
		}
		if p.cb.State() != StateHalfOpen {
			t.Fatal("canceled probe was counted as success")
		}
		if err := replacement(nil); err != nil {
			t.Fatal(err)
		}
		if p.cb.State() != StateClosed {
			t.Fatal("replacement success did not close the breaker")
		}
	})
}

func TestAdmissionOldGenerationCannotChangeBreaker(t *testing.T) {
	t.Parallel()

	for _, transition := range []string{"half-open", "recovered", "reset closed", "reset open"} {
		t.Run(transition, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				p := NewPolicy().WithCircuitBreaker(CircuitBreakerConfig{
					MaxFailures: 1, Timeout: time.Second, HalfOpenMaxCalls: 1,
				})
				_, old, err := p.Acquire(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				failure := errors.New("trip")
				want := StateClosed
				if transition != "reset closed" {
					_, current, err := p.Acquire(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					if err := current(failure); !errors.Is(err, failure) {
						t.Fatal(err)
					}
				}
				switch transition {
				case "reset closed", "reset open":
					p.cb.Reset()
				case "half-open", "recovered":
					<-time.After(time.Second)
					want = StateHalfOpen
					if transition == "recovered" {
						_, probe, err := p.Acquire(context.Background())
						if err != nil {
							t.Fatal(err)
						}
						if err := probe(nil); err != nil {
							t.Fatal(err)
						}
						want = StateClosed
					}
				}
				if err := old(failure); !errors.Is(err, failure) {
					t.Fatal(err)
				}
				if p.cb.State() != want {
					t.Fatalf("stale result changed breaker: got %s, want %s", p.cb.State(), want)
				}
			})
		})
	}
}

func TestAdmissionNilPolicyAndUnaryOutcome(t *testing.T) {
	t.Parallel()

	var p *Policy
	_, finish, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := finish(nil); err != nil {
		t.Fatal(err)
	}
	if err := finish(errors.New("late")); err != nil {
		t.Fatalf("successful terminal outcome changed: %v", err)
	}
	synctest.Test(t, func(t *testing.T) {
		p := NewPolicy().WithTimeout(10 * time.Millisecond)
		result, err := Execute(context.Background(), p, func(ctx context.Context) (int, error) {
			<-ctx.Done()
			return 1, nil
		})
		if result != 1 || err != nil {
			t.Fatalf("successful callback outcome was replaced: result=%d error=%v", result, err)
		}
	})
}

func TestAdmissionCancellationAfterSlotAcquired(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var releases atomic.Int32
	p := NewPolicy().
		WithBulkhead(BulkheadConfig{
			MaxConcurrent: 1,
			OnAcquire:     func(string) { cancel() },
			OnRelease:     func(string) { releases.Add(1) },
		}).
		WithCircuitBreaker(CircuitBreakerConfig{MaxFailures: 1})
	callCtx, finish, err := p.Acquire(ctx)
	if callCtx != nil || finish != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled admission escaped: %v", err)
	}
	if releases.Load() != 1 || !p.IsAvailable() {
		t.Fatal("canceled admission leaked a slot or changed breaker health")
	}
}

func TestAdmissionOldSuccessCannotCloseHalfOpenBreaker(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		p := NewPolicy().WithCircuitBreaker(CircuitBreakerConfig{
			MaxFailures: 1, Timeout: time.Second, HalfOpenMaxCalls: 1,
		})
		_, old, err := p.Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		_, trip, err := p.Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		failure := errors.New("trip")
		if err := trip(failure); !errors.Is(err, failure) {
			t.Fatal(err)
		}
		<-time.After(time.Second)
		if err := old(nil); err != nil {
			t.Fatal(err)
		}
		if p.cb.State() != StateHalfOpen {
			t.Fatal("old success certified recovery without a probe")
		}
	})
}

func TestAdmissionUnaryPanicIsNotSuccess(t *testing.T) {
	t.Parallel()

	for _, direct := range []bool{false, true} {
		t.Run(fmt.Sprintf("direct_breaker=%t", direct), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				p := NewPolicy().
					WithBulkhead(BulkheadConfig{MaxConcurrent: 1}).
					WithCircuitBreaker(CircuitBreakerConfig{
						MaxFailures: 1, Timeout: time.Second, HalfOpenMaxCalls: 1,
					})
				_, finish, err := p.Acquire(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				failure := errors.New("trip")
				if err := finish(failure); !errors.Is(err, failure) {
					t.Fatal(err)
				}
				<-time.After(time.Second)
				func() {
					defer func() {
						if recovered := recover(); recovered != "callback panic" {
							t.Errorf("panic did not propagate: %v", recovered)
						}
					}()
					if direct {
						_ = p.cb.Execute(func() error { panic("callback panic") })
					} else {
						_, _ = Execute(context.Background(), p, func(context.Context) (int, error) {
							panic("callback panic")
						})
					}
				}()
				if p.cb.State() != StateHalfOpen {
					t.Fatal("panic was counted as a successful recovery")
				}
				_, replacement, err := p.Acquire(context.Background())
				if err != nil {
					t.Fatalf("panic leaked probe or bulkhead capacity: %v", err)
				}
				if err := replacement(nil); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestAdmissionConcurrentBreakerTransition(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{MaxFailures: 1, Timeout: time.Second})
		failure := errors.New("trip")
		if err := cb.Execute(func() error { return failure }); !errors.Is(err, failure) {
			t.Fatal(err)
		}
		<-time.After(time.Second)
		var wg sync.WaitGroup
		for range 32 {
			wg.Go(func() {
				if cb.State() != StateHalfOpen {
					t.Error("expired breaker did not become half-open")
				}
			})
		}
		wg.Wait()
	})
}
