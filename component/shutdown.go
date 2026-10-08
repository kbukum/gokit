package component

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

// ShutdownPhase orders dependency release after all work has drained. Registration order is reversed within each phase.
type ShutdownPhase int

const (
	PhaseResources ShutdownPhase = iota
	PhaseTelemetry
	PhaseAdmin
)

// StopReserveDivisor sets the share of the shutdown deadline kept for stopping components and releasing resources: 1/StopReserveDivisor. Draining uses the rest.
const StopReserveDivisor = 4

// Quiescer stops acceptance immediately without waiting for accepted work.
type Quiescer interface {
	Quiesce() error
}

// DrainPhase orders draining before any dependency is closed.
type DrainPhase int

const (
	DrainIngress DrainPhase = iota
	DrainWorkers
)

// Drainer releases accepted work within its deadline. A listener must force-close at expiry; a worker must cancel and join cooperative handlers.
type Drainer interface {
	Drain(context.Context) error
	DrainPhase() DrainPhase
}

// QuiesceAll rejects new work before shutdown hooks or draining begin. Implementations must return promptly and be idempotent.
func (r *Registry) QuiesceAll() error {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	r.mu.RLock()
	var entries []*componentEntry
	for _, entry := range r.entries {
		if entry.state == StateRunning {
			entries = append(entries, entry)
		}
	}
	r.mu.RUnlock()
	var errs []error
	for _, entry := range quiesceOrder(entries) {
		c := entry.component
		if quiescer, ok := c.(Quiescer); ok {
			if err := quiescer.Quiesce(); err != nil {
				errs = append(errs, fmt.Errorf("quiesce %s: %w", c.Name(), err))
			}
		}
	}
	return errors.Join(errs...)
}

func quiesceOrder(entries []*componentEntry) []*componentEntry {
	ordered := slices.Clone(entries)
	priority := func(entry *componentEntry) int {
		if drainer, ok := entry.component.(Drainer); ok && drainer.DrainPhase() == DrainIngress {
			return 0
		}
		return 1
	}
	slices.SortStableFunc(ordered, func(a, b *componentEntry) int { return priority(a) - priority(b) })
	return ordered
}

// Shutdown quiesces every component, drains ingress then workers, and closes resources, telemetry, then admin. releaseResources closes container-owned clients after resource components and before telemetry.
//
// The deadline is split so that adding components never shortens draining: 1/[StopReserveDivisor] of it is reserved for stops and the release, and draining uses the rest. Drainers in one phase drain concurrently. Each phase present gets an equal part of the drain window left when it starts, so time ingress does not use passes to workers. Stops share whatever time remains, in order. Calls are cooperative, never detached.
func (r *Registry) Shutdown(ctx context.Context, releaseResources func(context.Context) error) error {
	var errs []error
	for _, result := range r.shutdown(ctx, releaseResources) {
		if result.Err != nil {
			errs = append(errs, fmt.Errorf("failed to stop %s: %w", result.Name, result.Err))
		}
	}
	return errors.Join(errs...)
}

func (r *Registry) shutdown(ctx context.Context, releaseResources func(context.Context) error) []StopResult {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	ctx, cancel := r.stopContext(ctx)
	defer cancel()

	r.mu.RLock()
	var entries []*componentEntry
	for i := len(r.entries) - 1; i >= 0; i-- {
		if r.entries[i].state == StateRunning {
			entries = append(entries, r.entries[i])
		}
	}
	r.mu.RUnlock()

	results := make([]StopResult, len(entries))
	indices := make(map[*componentEntry]int, len(entries))
	var drains []*componentEntry
	for i, entry := range entries {
		results[i].Name = entry.component.Name()
		indices[entry] = i
		if _, ok := entry.component.(Drainer); ok {
			drains = append(drains, entry)
		}
	}
	for _, entry := range quiesceOrder(entries) {
		if quiescer, ok := entry.component.(Quiescer); ok {
			results[indices[entry]].Err = quiescer.Quiesce()
		}
	}
	deadline, _ := ctx.Deadline()
	drainEnd := deadline.Add(-time.Until(deadline) / StopReserveDivisor)
	for _, phase := range drainPhases(drains) {
		phaseEnd := time.Now().Add(max(time.Until(drainEnd), 0) / time.Duration(phase.remaining))
		drainPhase(ctx, phaseEnd, phase.entries, indices, results)
	}
	remaining := len(entries)
	if releaseResources != nil {
		remaining++
	}
	call := func(fn func(context.Context) error) error {
		opCtx, opCancel := context.WithTimeout(ctx, max(time.Until(deadline)/time.Duration(max(remaining, 1)), 0))
		defer opCancel()
		remaining--
		return fn(opCtx)
	}
	slices.SortStableFunc(entries, func(a, b *componentEntry) int { return int(a.phase - b.phase) })
	release := func() {
		if releaseResources != nil {
			results = append(results, StopResult{Name: "container", Err: call(releaseResources)})
			releaseResources = nil
		}
	}
	for _, entry := range entries {
		if entry.phase > PhaseResources {
			release()
		}
		r.mu.Lock()
		entry.state = StateStopping
		r.mu.Unlock()
		i := indices[entry]
		results[i].Err = errors.Join(results[i].Err, call(entry.component.Stop))
		r.mu.Lock()
		entry.state = StateStopped
		r.mu.Unlock()
	}
	release()
	return results
}

type drainGroup struct {
	entries []*componentEntry
	// remaining counts this phase and the phases after it, which share the drain window left when it starts.
	remaining int
}

// drainPhases groups drainers by phase, ingress first.
func drainPhases(drains []*componentEntry) []drainGroup {
	byPhase := map[DrainPhase][]*componentEntry{}
	var phases []DrainPhase
	for _, entry := range drains {
		drainer, _ := entry.component.(Drainer)
		p := drainer.DrainPhase()
		if _, seen := byPhase[p]; !seen {
			phases = append(phases, p)
		}
		byPhase[p] = append(byPhase[p], entry)
	}
	slices.Sort(phases)
	groups := make([]drainGroup, len(phases))
	for i, p := range phases {
		groups[i] = drainGroup{entries: byPhase[p], remaining: len(phases) - i}
	}
	return groups
}

// drainPhase drains every entry concurrently until each returns or phaseEnd passes. Each result slot is written by one goroutine and read after all have joined.
func drainPhase(ctx context.Context, phaseEnd time.Time, entries []*componentEntry, indices map[*componentEntry]int, results []StopResult) {
	phaseCtx, cancel := context.WithDeadline(ctx, phaseEnd)
	defer cancel()
	var wg sync.WaitGroup
	for _, entry := range entries {
		drainer, _ := entry.component.(Drainer)
		i := indices[entry]
		wg.Go(func() {
			results[i].Err = errors.Join(results[i].Err, drainer.Drain(phaseCtx))
		})
	}
	wg.Wait()
}
