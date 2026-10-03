package component

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

// ShutdownPhase orders dependency release after all work has drained. Registration order is reversed within each phase.
type ShutdownPhase int

const (
	PhaseResources ShutdownPhase = iota
	PhaseTelemetry
	PhaseAdmin
)

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

// Shutdown quiesces every component, drains ingress then workers, and closes resources, telemetry, then admin. releaseResources closes container-owned clients after resource components and before telemetry. Each operation receives a share of the remaining total deadline, so draining cannot consume the entire shutdown budget. Calls are cooperative, never detached.
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
	slices.SortStableFunc(drains, func(a, b *componentEntry) int {
		ad, _ := a.component.(Drainer)
		bd, _ := b.component.(Drainer)
		return int(ad.DrainPhase() - bd.DrainPhase())
	})
	remaining := len(entries) + len(drains)
	if releaseResources != nil {
		remaining++
	}
	call := func(fn func(context.Context) error) error {
		deadline, _ := ctx.Deadline()
		opCtx, opCancel := context.WithTimeout(ctx, max(time.Until(deadline)/time.Duration(max(remaining, 1)), 0))
		defer opCancel()
		remaining--
		return fn(opCtx)
	}
	for _, entry := range drains {
		drainer, _ := entry.component.(Drainer)
		i := indices[entry]
		results[i].Err = errors.Join(results[i].Err, call(drainer.Drain))
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
