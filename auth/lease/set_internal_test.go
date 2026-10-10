package lease

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/util"
)

func TestLateAnswerCannotRenewBeforeExpiryWorkerRuns(t *testing.T) {
	clock := util.NewFakeClock(time.Now())
	life, cancel := context.WithCancelCause(context.Background())
	e := &entry[string]{key: "a", deadline: clock.Now().Add(time.Second), end: clock.Now().Add(time.Hour), cancel: cancel, stop: func() bool { return true }}
	s := &Set[string]{timing: clock, wake: make(chan struct{}, 1), leases: map[*entry[string]]struct{}{e: {}}}
	s.cfg = Config[string]{Lease: 3 * time.Second, Checker: CheckerFunc[string](func(context.Context, []string) (map[string]bool, error) {
		clock.Advance(2 * time.Second)
		return map[string]bool{"a": true}, nil
	})}
	if err := s.renew(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(context.Cause(life), ErrExpired) || s.Len() != 0 {
		t.Fatal("late reply resurrected an expired but still indexed lease")
	}
}

func TestTimelyPartialAnswersApplyAlongsideError(t *testing.T) {
	clock := util.NewFakeClock(time.Now())
	unavailable := errors.New("other authority unavailable")
	first, stopFirst := context.WithCancelCause(context.Background())
	second, stopSecond := context.WithCancelCause(context.Background())
	a := &entry[string]{key: "a", deadline: clock.Now().Add(time.Second), end: clock.Now().Add(time.Hour), cancel: stopFirst, stop: func() bool { return true }}
	b := &entry[string]{key: "b", deadline: a.deadline, end: a.end, cancel: stopSecond, stop: func() bool { return true }}
	s := &Set[string]{timing: clock, wake: make(chan struct{}, 1), leases: map[*entry[string]]struct{}{a: {}, b: {}}}
	s.cfg = Config[string]{Lease: 3 * time.Second, Checker: CheckerFunc[string](func(context.Context, []string) (map[string]bool, error) {
		return map[string]bool{"a": true}, unavailable
	})}
	if err := s.renew(context.Background()); !errors.Is(err, unavailable) {
		t.Fatal("partial error lost")
	}
	if a.deadline.Sub(clock.Now()) != 3*time.Second || b.deadline.Sub(clock.Now()) != time.Second || first.Err() != nil || second.Err() != nil {
		t.Fatal("partial response did not preserve independent authority results")
	}
	stopFirst(ErrReleased)
	stopSecond(ErrReleased)
}
