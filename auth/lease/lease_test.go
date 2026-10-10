package lease_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kbukum/gokit/auth/lease"
	apperrors "github.com/kbukum/gokit/errors"
)

// checker answers every batch from a table the test controls and records the keys it received.
type checker struct {
	mu      sync.Mutex
	answers map[string]bool
	err     error
	block   chan struct{}
	batches [][]string
	entered chan struct{}
}

func newChecker() *checker {
	return &checker{answers: map[string]bool{}, entered: make(chan struct{}, 16)}
}

func (c *checker) set(key string, ok bool) { c.mu.Lock(); c.answers[key] = ok; c.mu.Unlock() }
func (c *checker) forget(key string)       { c.mu.Lock(); delete(c.answers, key); c.mu.Unlock() }

func (c *checker) Check(ctx context.Context, keys []string) (map[string]bool, error) {
	c.mu.Lock()
	c.batches = append(c.batches, slices.Clone(keys))
	block, err := c.block, c.err
	c.mu.Unlock()
	select {
	case c.entered <- struct{}{}:
	default:
	}
	if block != nil {
		<-block // models a driver that ignores cancellation
	}
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]bool{}
	for _, k := range keys {
		if v, ok := c.answers[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}

func config(c lease.Checker[string]) lease.Config[string] {
	return lease.Config[string]{Checker: c, Lease: 3 * time.Second, Interval: time.Second, Limit: 4, ReportError: func(context.Context, error) {}}
}

func newSet(t *testing.T, c lease.Checker[string]) *lease.Set[string] {
	t.Helper()
	s, err := lease.New(config(c))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return s
}

func acquire(t *testing.T, s *lease.Set[string], key string, lifetime time.Duration) (life context.Context, release func()) {
	t.Helper()
	life, release, err := s.Acquire(context.Background(), key, lease.Grant{CheckedAt: time.Now(), Lifetime: lifetime})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return life, release
}

func endedWith(t *testing.T, life context.Context, want error) {
	t.Helper()
	synctest.Wait()
	if life.Err() == nil {
		t.Fatal("lease still alive")
	}
	if got := context.Cause(life); !errors.Is(got, want) {
		t.Fatalf("cause = %v, want %v", got, want)
	}
}

func alive(t *testing.T, life context.Context) {
	t.Helper()
	synctest.Wait()
	if life.Err() != nil {
		t.Fatalf("lease ended early: %v", context.Cause(life))
	}
}

func requireReason(t *testing.T, err error, code apperrors.ErrorCode, reason string) {
	t.Helper()
	app, ok := apperrors.AsAppError(err)
	if !ok || app.Code != code || app.Reason != reason {
		t.Fatalf("got %v, want %s/%s", err, code, reason)
	}
}

func TestConfigValidation(t *testing.T) {
	t.Parallel()
	good := config(newChecker())
	cases := map[string]func(*lease.Config[string]){
		"nil checker":          func(c *lease.Config[string]) { c.Checker = nil },
		"nil error reporter":   func(c *lease.Config[string]) { c.ReportError = nil },
		"zero lease":           func(c *lease.Config[string]) { c.Lease = 0 },
		"zero interval":        func(c *lease.Config[string]) { c.Interval = 0 },
		"interval at lease":    func(c *lease.Config[string]) { c.Interval = c.Lease },
		"zero limit":           func(c *lease.Config[string]) { c.Limit = 0 },
		"limit over the bound": func(c *lease.Config[string]) { c.Limit = lease.MaxLimit + 1 },
	}
	for name, mutate := range cases {
		cfg := good
		mutate(&cfg)
		if _, err := lease.New(cfg); err == nil {
			t.Errorf("%s: accepted", name)
		} else {
			requireReason(t, err, apperrors.ErrCodeInvalidInput, "")
		}
	}
}

func TestRenewalKeepsAuthorizedLeasesAlive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newChecker()
		c.set("a", true)
		s := newSet(t, c)
		life, _ := acquire(t, s, "a", time.Hour)
		time.Sleep(30 * time.Second)
		alive(t, life)
		c.mu.Lock()
		n := len(c.batches)
		c.mu.Unlock()
		if n < 29 || n > 31 {
			t.Fatalf("expected one batch per interval, got %d", n)
		}
	})
}

func TestDeniedKeyEndsAtNextBatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newChecker()
		c.set("a", true)
		s := newSet(t, c)
		life, _ := acquire(t, s, "a", time.Hour)
		time.Sleep(2 * time.Second)
		alive(t, life)
		c.set("a", false)
		time.Sleep(time.Second)
		endedWith(t, life, lease.ErrRevoked)
		if s.Len() != 0 {
			t.Fatal("ended lease still counted")
		}
	})
}

func TestUnansweredKeyAndBatchErrorExpireFromCheckStart(t *testing.T) {
	for _, mode := range []string{"unanswered", "batch error"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c := newChecker()
				if mode == "batch error" {
					c.set("a", true)
					c.err = errors.New("authority unavailable")
				}
				s := newSet(t, c)
				start := time.Now()
				life, _ := acquire(t, s, "a", time.Hour)
				time.Sleep(3*time.Second - time.Nanosecond)
				alive(t, life)
				time.Sleep(time.Nanosecond)
				endedWith(t, life, lease.ErrExpired)
				if time.Since(start) != 3*time.Second {
					t.Fatal("expiry not measured from the admitting check")
				}
			})
		})
	}
}

func TestExpiryIsIndependentOfBlockedChecker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newChecker()
		c.set("a", true)
		s := newSet(t, c)
		life, _ := acquire(t, s, "a", time.Hour)
		time.Sleep(time.Second)
		synctest.Wait()
		c.mu.Lock()
		c.block = make(chan struct{})
		block := c.block
		c.mu.Unlock()
		<-c.entered // the renewal at one second succeeded
		<-c.entered // the renewal at two seconds is now stuck
		time.Sleep(3 * time.Second)
		endedWith(t, life, lease.ErrExpired)
		close(block)
	})
}

func TestRenewalExtendsFromCheckStartNotCompletion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		var calls int
		var mu sync.Mutex
		// Only the first check (the manual one at t=0) answers, after a 2.5 s stall; later checks answer nothing.
		probe := lease.CheckerFunc[string](func(ctx context.Context, keys []string) (map[string]bool, error) {
			mu.Lock()
			calls++
			first := calls == 1
			mu.Unlock()
			if !first {
				return map[string]bool{}, nil
			}
			<-release
			return map[string]bool{"a": true}, nil
		})
		s := newSet(t, probe)
		start := time.Now()
		life, _ := acquire(t, s, "a", time.Hour)
		s.Nudge()
		time.Sleep(2500 * time.Millisecond)
		close(release)
		synctest.Wait()
		time.Sleep(time.Until(start.Add(3*time.Second - time.Nanosecond)))
		alive(t, life)
		time.Sleep(time.Nanosecond)
		endedWith(t, life, lease.ErrExpired)
	})
}

func TestFutureCheckRejected(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSet(t, newChecker())
		_, _, err := s.Acquire(context.Background(), "a", lease.Grant{CheckedAt: time.Now().Add(time.Nanosecond), Lifetime: time.Hour})
		requireReason(t, err, apperrors.ErrCodeInvalidInput, "")
	})
}

func TestLifetimeIsNotRenewable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newChecker()
		c.set("a", true)
		s := newSet(t, c)
		life, _ := acquire(t, s, "a", 5*time.Second)
		time.Sleep(5*time.Second - time.Nanosecond)
		alive(t, life)
		time.Sleep(time.Nanosecond)
		endedWith(t, life, lease.ErrEnded)
	})
}

func TestBatchHoldsEachKeyOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newChecker()
		c.set("a", true)
		c.set("b", true)
		s := newSet(t, c)
		acquire(t, s, "a", time.Hour)
		acquire(t, s, "a", time.Hour)
		acquire(t, s, "b", time.Hour)
		s.Nudge()
		synctest.Wait()
		c.mu.Lock()
		last := slices.Sorted(slices.Values(c.batches[len(c.batches)-1]))
		c.mu.Unlock()
		if !slices.Equal(last, []string{"a", "b"}) {
			t.Fatalf("batch = %v", last)
		}
	})
}

func TestCheckerContextIsBoundedByLease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		var deadline time.Time
		probe := lease.CheckerFunc[string](func(ctx context.Context, keys []string) (map[string]bool, error) {
			mu.Lock()
			defer mu.Unlock()
			deadline, _ = ctx.Deadline()
			return map[string]bool{}, nil
		})
		s := newSet(t, probe)
		acquire(t, s, "a", time.Hour)
		start := time.Now()
		s.Nudge()
		synctest.Wait()
		mu.Lock()
		defer mu.Unlock()
		if !deadline.Equal(start.Add(3 * time.Second)) {
			t.Fatalf("check deadline %v, want lease from check start", deadline.Sub(start))
		}
	})
}

func TestAcquireRules(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSet(t, newChecker())
		_, _, err := s.Acquire(context.Background(), "a", lease.Grant{CheckedAt: time.Now().Add(-3 * time.Second), Lifetime: time.Hour})
		requireReason(t, err, apperrors.ErrCodeUnauthorized, lease.ReasonExpired)
		_, _, err = s.Acquire(context.Background(), "a", lease.Grant{CheckedAt: time.Now(), Lifetime: 0})
		requireReason(t, err, apperrors.ErrCodeUnauthorized, lease.ReasonExpired)
		canceled, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, err = s.Acquire(canceled, "a", lease.Grant{CheckedAt: time.Now(), Lifetime: time.Hour})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled owner: %v", err)
		}
		releases := make([]func(), 0, 4)
		for range 4 {
			_, release := acquire(t, s, "a", time.Hour)
			releases = append(releases, release)
		}
		_, _, err = s.Acquire(context.Background(), "a", lease.Grant{CheckedAt: time.Now(), Lifetime: time.Hour})
		requireReason(t, err, apperrors.ErrCodeRateLimited, lease.ReasonCapacity)
		releases[0]()
		releases[0]()
		if s.Len() != 3 {
			t.Fatalf("release not idempotent: %d", s.Len())
		}
		acquire(t, s, "a", time.Hour)
	})
}

func TestReleaseOwnerCancelAndLateResults(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newChecker()
		c.set("a", true)
		s := newSet(t, c)
		life, release := acquire(t, s, "a", time.Hour)
		release()
		endedWith(t, life, lease.ErrReleased)

		owner, cancel := context.WithCancel(context.Background())
		life, _, err := s.Acquire(owner, "a", lease.Grant{CheckedAt: time.Now(), Lifetime: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		endedWith(t, life, context.Canceled)
		if s.Len() != 0 {
			t.Fatal("owner-canceled lease still counted")
		}

		c.mu.Lock()
		c.block = make(chan struct{})
		block := c.block
		c.mu.Unlock()
		life, release = acquire(t, s, "a", time.Hour)
		s.Nudge()
		<-c.entered
		release()
		c.mu.Lock()
		c.block = nil
		c.mu.Unlock()
		close(block)
		synctest.Wait()
		if s.Len() != 0 || !errors.Is(context.Cause(life), lease.ErrReleased) {
			t.Fatal("late renewal resurrected a released lease")
		}
	})
}

func TestRevokeMatchesKeys(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newChecker()
		c.set("a", true)
		c.set("b", true)
		s := newSet(t, c)
		a, _ := acquire(t, s, "a", time.Hour)
		b, _ := acquire(t, s, "b", time.Hour)
		if n := s.Revoke(func(k string) bool { return k == "a" }); n != 1 {
			t.Fatalf("revoked %d", n)
		}
		if a.Err() == nil {
			t.Fatal("revoke did not end the lease before returning")
		}
		endedWith(t, a, lease.ErrRevoked)
		alive(t, b)
	})
}

func TestRevokePredicateRunsOutsideTheSetLock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newChecker()
		c.set("a", true)
		s := newSet(t, c)
		a, _ := acquire(t, s, "a", time.Hour)
		if n := s.Revoke(func(string) bool { return s.Len() == 1 }); n != 1 {
			t.Fatalf("revoked %d", n)
		}
		endedWith(t, a, lease.ErrRevoked)
	})
}

func TestCloseEndsLeasesAndRejectsNewOnes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, err := lease.New(config(newChecker()))
		if err != nil {
			t.Fatal(err)
		}
		life, release, err := s.Acquire(context.Background(), "a", lease.Grant{CheckedAt: time.Now(), Lifetime: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if err := s.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
		if life.Err() == nil || !errors.Is(context.Cause(life), lease.ErrClosed) {
			t.Fatalf("close cause: %v", context.Cause(life))
		}
		_, _, err = s.Acquire(context.Background(), "a", lease.Grant{CheckedAt: time.Now(), Lifetime: time.Hour})
		requireReason(t, err, apperrors.ErrCodeServiceUnavailable, lease.ReasonClosed)
		if err := s.Close(context.Background()); err != nil {
			t.Fatal("close is not idempotent")
		}
	})
}

func TestCloseIsBoundedByContextWhenCheckerHangs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newChecker()
		c.block = make(chan struct{})
		s, err := lease.New(config(c))
		if err != nil {
			t.Fatal(err)
		}
		_, release, err := s.Acquire(context.Background(), "a", lease.Grant{CheckedAt: time.Now(), Lifetime: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		<-c.entered
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := s.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("close with hung checker: %v", err)
		}
		close(c.block)
		if err := s.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}
