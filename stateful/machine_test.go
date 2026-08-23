package stateful

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type orderState int

const (
	orderDraft orderState = iota
	orderSubmitted
)

type countingPersistence struct {
	count atomic.Int64
}

func (c *countingPersistence) Persist(_ StateSnapshot[orderState], _ AuditEntry[orderState, bool]) error {
	c.count.Add(1)
	return nil
}

type failingPersistence struct{}

var errPersist = errors.New("persist failed")

func (failingPersistence) Persist(_ StateSnapshot[orderState], _ AuditEntry[orderState, bool]) error {
	return errPersist
}

func fixedClock(t time.Time) MachineOption[orderState, bool] {
	return WithClock[orderState, bool](func() time.Time { return t })
}

func TestStateMachineGuardedTransitionUpdatesStateAndAudit(t *testing.T) {
	t.Parallel()

	persistence := &countingPersistence{}
	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	machine := NewStateMachine[orderState, bool](orderDraft, fixedClock(at)).
		WithTransition(
			NewTransition[orderState, bool]("submit", orderSubmitted).
				From(orderDraft).
				WithGuard(func(_ orderState, allowed bool) error {
					if allowed {
						return nil
					}
					return errors.New("not allowed")
				}),
		).
		WithPersistence(persistence)

	snapshot, err := machine.Apply("submit", true)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if snapshot.State != orderSubmitted {
		t.Errorf("state = %v, want %v", snapshot.State, orderSubmitted)
	}
	if snapshot.Version != 1 {
		t.Errorf("version = %d, want 1", snapshot.Version)
	}

	log := machine.AuditLog()
	if len(log) != 1 {
		t.Fatalf("audit log len = %d, want 1", len(log))
	}
	entry := log[0]
	if entry.Transition != "submit" || entry.From != orderDraft || entry.To != orderSubmitted {
		t.Errorf("unexpected audit entry: %+v", entry)
	}
	if entry.Version != 1 {
		t.Errorf("audit version = %d, want 1", entry.Version)
	}
	if !entry.RecordedAt.Equal(at) {
		t.Errorf("recordedAt = %v, want %v", entry.RecordedAt, at)
	}
	if got := persistence.count.Load(); got != 1 {
		t.Errorf("persist count = %d, want 1", got)
	}
	if machine.State() != orderSubmitted {
		t.Errorf("machine state = %v, want %v", machine.State(), orderSubmitted)
	}
}

func TestStateMachineGuardRejectionAbortsTransition(t *testing.T) {
	t.Parallel()

	machine := NewStateMachine[orderState, bool](orderDraft).
		WithTransition(
			NewTransition[orderState, bool]("submit", orderSubmitted).
				From(orderDraft).
				WithGuard(func(_ orderState, allowed bool) error {
					if allowed {
						return nil
					}
					return errors.New("not allowed")
				}),
		)

	if _, err := machine.Apply("submit", false); err == nil {
		t.Fatal("expected guard rejection error")
	}
	if machine.State() != orderDraft {
		t.Errorf("state = %v, want %v", machine.State(), orderDraft)
	}
	if machine.Snapshot().Version != 0 {
		t.Errorf("version = %d, want 0", machine.Snapshot().Version)
	}
	if len(machine.AuditLog()) != 0 {
		t.Errorf("audit log not empty: %v", machine.AuditLog())
	}
}

func TestStateMachineActionFailureDoesNotChangeStateOrAudit(t *testing.T) {
	t.Parallel()

	machine := NewStateMachine[orderState, bool](orderDraft).
		WithTransition(
			NewTransition[orderState, bool]("submit", orderSubmitted).
				From(orderDraft).
				WithAction(func(_, _ orderState, _ bool) error {
					return errors.New("action failed")
				}),
		)

	if _, err := machine.Apply("submit", true); err == nil {
		t.Fatal("expected action failure error")
	}
	if machine.State() != orderDraft {
		t.Errorf("state = %v, want %v", machine.State(), orderDraft)
	}
	if machine.Snapshot().Version != 0 {
		t.Errorf("version = %d, want 0", machine.Snapshot().Version)
	}
	if len(machine.AuditLog()) != 0 {
		t.Errorf("audit log not empty")
	}
}

func TestStateMachinePersistenceFailureDoesNotChangeStateOrAudit(t *testing.T) {
	t.Parallel()

	machine := NewStateMachine[orderState, bool](orderDraft).
		WithTransition(NewTransition[orderState, bool]("submit", orderSubmitted).From(orderDraft)).
		WithPersistence(failingPersistence{})

	_, err := machine.Apply("submit", true)
	if !errors.Is(err, errPersist) {
		t.Fatalf("error = %v, want wrap of errPersist", err)
	}
	if machine.State() != orderDraft {
		t.Errorf("state = %v, want %v", machine.State(), orderDraft)
	}
	if machine.Snapshot().Version != 0 {
		t.Errorf("version = %d, want 0", machine.Snapshot().Version)
	}
	if len(machine.AuditLog()) != 0 {
		t.Errorf("audit log not empty")
	}
}

func TestStateMachineUnregisteredAndWrongSourceRejected(t *testing.T) {
	t.Parallel()

	transition := NewTransition[orderState, bool]("submit", orderSubmitted).From(orderSubmitted)
	if transition.Name() != "submit" {
		t.Errorf("name = %q, want submit", transition.Name())
	}
	machine := NewStateMachine[orderState, bool](orderDraft).WithTransition(transition)

	if _, err := machine.Apply("missing", true); !errors.Is(err, ErrTransitionNotRegistered) {
		t.Errorf("missing transition error = %v, want ErrTransitionNotRegistered", err)
	}
	if _, err := machine.Apply("submit", true); !errors.Is(err, ErrTransitionNotAllowed) {
		t.Errorf("wrong-source error = %v, want ErrTransitionNotAllowed", err)
	}
}

func TestStateMachineAnySourceTransition(t *testing.T) {
	t.Parallel()

	machine := NewStateMachine[orderState, bool](orderSubmitted).
		WithTransition(NewTransition[orderState, bool]("reset", orderDraft))

	snapshot, err := machine.Apply("reset", true)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if snapshot.State != orderDraft {
		t.Errorf("state = %v, want %v", snapshot.State, orderDraft)
	}
}

func TestStateMachineConcurrentApplyIsSerialized(t *testing.T) {
	t.Parallel()

	machine := NewStateMachine[int, bool](0).
		WithTransition(NewTransition[int, bool]("bump", 1))

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = machine.Apply("bump", true)
		}()
	}
	wg.Wait()

	if got := machine.Snapshot().Version; got != 50 {
		t.Errorf("version = %d, want 50", got)
	}
	if got := len(machine.AuditLog()); got != 50 {
		t.Errorf("audit log len = %d, want 50", got)
	}
}

func TestStateMachineMaxAuditEntriesEvictsOldest(t *testing.T) {
	t.Parallel()

	machine := NewStateMachine[int, bool](0,
		WithMaxAuditEntries[int, bool](3)).
		WithTransition(NewTransition[int, bool]("bump", 1))

	for range 10 {
		if _, err := machine.Apply("bump", true); err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}

	log := machine.AuditLog()
	if len(log) != 3 {
		t.Fatalf("audit log len = %d, want 3", len(log))
	}
	// The retained entries must be the most recent ones (versions 8, 9, 10).
	for i, want := range []uint64{8, 9, 10} {
		if log[i].Version != want {
			t.Errorf("log[%d].Version = %d, want %d", i, log[i].Version, want)
		}
	}
	if got := machine.Snapshot().Version; got != 10 {
		t.Errorf("version = %d, want 10", got)
	}
}
