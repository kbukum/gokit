package dag

import (
	"sync"
	"time"
)

// Session holds per-session state for streaming pipelines.
type Session struct {
	// ID is the session identifier.
	ID string
	// State is the shared state across execution cycles.
	State *State

	mu        sync.Mutex
	schedules map[string]*scheduleState
}

// NewSession creates a new streaming session.
func NewSession(id string) *Session {
	return &Session{
		ID:        id,
		State:     NewState(),
		schedules: make(map[string]*scheduleState),
	}
}

type scheduleState struct {
	lastRun   time.Time
	firstSeen time.Time
}

// ReadyFilter returns a NodeFilter that checks schedule + conditions.
// A node is ready if:
//   - It has no schedule (always ready), OR
//   - Its schedule interval has elapsed AND its min_buffer period has passed
//   - AND its condition function (if any) returns true
func (s *Session) ReadyFilter(pipeline *Pipeline, conditions map[string]ConditionFunc) NodeFilter {
	// Build a lookup map: component -> NodeDef
	nodeDefs := make(map[string]NodeDef)
	for _, def := range pipeline.Nodes {
		nodeDefs[def.Component] = def
	}

	return func(nodeName string, state *State) bool {
		s.mu.Lock()
		defer s.mu.Unlock()

		def, ok := nodeDefs[nodeName]
		if !ok {
			return true // not defined in pipeline, run it
		}

		// Check condition
		if def.Condition != "" && conditions != nil {
			condFn, exists := conditions[def.Condition]
			if exists && !condFn(state) {
				return false
			}
		}

		// Check schedule
		if def.Schedule == nil {
			return true // no schedule, always ready
		}

		now := time.Now()
		sched, exists := s.schedules[nodeName]
		if !exists {
			sched = &scheduleState{firstSeen: now}
			s.schedules[nodeName] = sched
		}

		// Check min_buffer (must accumulate data before first run)
		if def.Schedule.MinBuffer > 0 && now.Sub(sched.firstSeen) < def.Schedule.MinBuffer {
			return false
		}

		// Check interval
		if def.Schedule.Interval > 0 && !sched.lastRun.IsZero() && now.Sub(sched.lastRun) < def.Schedule.Interval {
			return false
		}

		// Mark as run
		sched.lastRun = now
		return true
	}
}

// ConditionFunc evaluates whether a node should run based on state.
type ConditionFunc func(state *State) bool
