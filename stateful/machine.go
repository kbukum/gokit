package stateful

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrTransitionNotRegistered is returned by StateMachine.Apply when no transition
// with the requested name is registered.
var ErrTransitionNotRegistered = errors.New("stateful: transition not registered")

// ErrTransitionNotAllowed is returned by StateMachine.Apply when a transition is
// restricted to a source state that does not match the machine's current state.
var ErrTransitionNotAllowed = errors.New("stateful: transition not allowed from current state")

// Transition is a named move from an optional source state to a target state,
// with an optional guard and action. S is the state type (compared for source
// matching) and C is the caller-supplied transition context.
type Transition[S comparable, C any] struct {
	name   string
	from   *S
	to     S
	guard  func(from S, ctx C) error
	action func(from, to S, ctx C) error
}

// NewTransition creates a transition that may be applied from any current state.
func NewTransition[S comparable, C any](name string, to S) Transition[S, C] {
	return Transition[S, C]{name: name, to: to}
}

// From restricts this transition to a specific source state.
func (t Transition[S, C]) From(state S) Transition[S, C] {
	t.from = &state
	return t
}

// WithGuard adds a guard that must return nil for the transition to proceed.
func (t Transition[S, C]) WithGuard(guard func(from S, ctx C) error) Transition[S, C] {
	t.guard = guard
	return t
}

// WithAction adds an action that runs after guard validation and before the
// transition commits. Returning an error aborts the transition without changing
// state or recording audit.
func (t Transition[S, C]) WithAction(action func(from, to S, ctx C) error) Transition[S, C] {
	t.action = action
	return t
}

// Name returns the transition name.
func (t Transition[S, C]) Name() string { return t.name }

// StateSnapshot is a point-in-time view of a StateMachine.
type StateSnapshot[S comparable] struct {
	// State is the current state.
	State S `json:"state"`
	// Version is the monotonic state version, incremented on each transition.
	Version uint64 `json:"version"`
}

// AuditEntry records one successfully applied transition.
type AuditEntry[S comparable, C any] struct {
	// Transition is the applied transition's name.
	Transition string `json:"transition"`
	// From is the state before the transition.
	From S `json:"from"`
	// To is the state after the transition.
	To S `json:"to"`
	// Context is the caller-supplied transition context.
	Context C `json:"context"`
	// Version is the state version after the transition.
	Version uint64 `json:"version"`
	// RecordedAt is the wall-clock time the transition was applied.
	RecordedAt time.Time `json:"recorded_at"`
}

// StatePersistence is a hook invoked for each successful transition, before the
// machine commits the new state. Returning an error aborts the transition.
type StatePersistence[S comparable, C any] interface {
	Persist(snapshot StateSnapshot[S], audit AuditEntry[S, C]) error
}

// MachineOption configures a StateMachine at construction.
type MachineOption[S comparable, C any] func(*StateMachine[S, C])

// WithClock injects the clock used for audit timestamps. Defaults to time.Now.
// Tests inject a fixed function for determinism.
func WithClock[S comparable, C any](now func() time.Time) MachineOption[S, C] {
	return func(m *StateMachine[S, C]) {
		if now != nil {
			m.now = now
		}
	}
}

// WithMaxAuditEntries bounds the in-memory audit log to the most recent n entries; older
// entries are evicted as new transitions are recorded. A value <= 0 (the default) keeps the
// log unbounded, which suits short-lived machines but grows without limit for long-lived
// ones — pair a bound with a StatePersistence hook when durable history is required.
func WithMaxAuditEntries[S comparable, C any](n int) MachineOption[S, C] {
	return func(m *StateMachine[S, C]) {
		if n > 0 {
			m.maxAudit = n
		}
	}
}

// StateMachine is a typed state machine with guarded transitions, an audit log,
// and persistence hooks. It is safe for concurrent use; Apply is serialized.
//
// The transition mutex is held while guards, actions, and persistence hooks run, so a
// callback must not call back into the same machine (State, Snapshot, AuditLog, Apply, …):
// the mutex is not reentrant and doing so deadlocks. Keep hooks self-contained and move any
// re-entrant work outside the callback.
type StateMachine[S comparable, C any] struct {
	mu          sync.Mutex
	current     S
	version     uint64
	auditLog    []AuditEntry[S, C]
	maxAudit    int
	transitions map[string]Transition[S, C]
	persistence []StatePersistence[S, C]
	now         func() time.Time
}

// NewStateMachine creates a state machine with the given initial state.
func NewStateMachine[S comparable, C any](initial S, opts ...MachineOption[S, C]) *StateMachine[S, C] {
	m := &StateMachine[S, C]{
		current:     initial,
		transitions: make(map[string]Transition[S, C]),
		now:         time.Now,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// WithTransition registers a transition and returns the receiver for chaining.
func (m *StateMachine[S, C]) WithTransition(t Transition[S, C]) *StateMachine[S, C] {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transitions[t.name] = t
	return m
}

// WithPersistence registers a persistence hook and returns the receiver for chaining.
func (m *StateMachine[S, C]) WithPersistence(p StatePersistence[S, C]) *StateMachine[S, C] {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.persistence = append(m.persistence, p)
	return m
}

// State returns the current state.
func (m *StateMachine[S, C]) State() S {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

// Snapshot returns the current state and version.
func (m *StateMachine[S, C]) Snapshot() StateSnapshot[S] {
	m.mu.Lock()
	defer m.mu.Unlock()
	return StateSnapshot[S]{State: m.current, Version: m.version}
}

// AuditLog returns a copy of the audit log.
func (m *StateMachine[S, C]) AuditLog() []AuditEntry[S, C] {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]AuditEntry[S, C](nil), m.auditLog...)
}

// Apply runs the named transition with the given context. On success it returns
// the new snapshot; the state, version, and audit log are committed atomically.
// A guard, action, or persistence failure aborts the transition and leaves the
// machine unchanged.
func (m *StateMachine[S, C]) Apply(name string, ctx C) (StateSnapshot[S], error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var zero StateSnapshot[S]
	transition, ok := m.transitions[name]
	if !ok {
		return zero, fmt.Errorf("%w: %q", ErrTransitionNotRegistered, name)
	}

	from := m.current
	if transition.from != nil && *transition.from != from {
		return zero, fmt.Errorf("%w: transition %q", ErrTransitionNotAllowed, name)
	}
	if transition.guard != nil {
		if err := transition.guard(from, ctx); err != nil {
			return zero, fmt.Errorf("stateful: transition %q guard rejected: %w", name, err)
		}
	}

	to := transition.to
	nextVersion := m.version + 1
	snapshot := StateSnapshot[S]{State: to, Version: nextVersion}

	if transition.action != nil {
		if err := transition.action(from, to, ctx); err != nil {
			return zero, fmt.Errorf("stateful: transition %q action failed: %w", name, err)
		}
	}

	audit := AuditEntry[S, C]{
		Transition: name,
		From:       from,
		To:         to,
		Context:    ctx,
		Version:    nextVersion,
		RecordedAt: m.now(),
	}

	for _, p := range m.persistence {
		if err := p.Persist(snapshot, audit); err != nil {
			return zero, fmt.Errorf("stateful: transition %q persistence failed: %w", name, err)
		}
	}

	m.current = to
	m.version = nextVersion
	m.auditLog = append(m.auditLog, audit)
	if m.maxAudit > 0 && len(m.auditLog) > m.maxAudit {
		drop := len(m.auditLog) - m.maxAudit
		m.auditLog = append(m.auditLog[:0], m.auditLog[drop:]...)
	}
	return snapshot, nil
}
