package main

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/stream"
)

// ConfigChange is a single configuration revision fanned out to every subscriber.
// It is deliberately trivial: the demo is about the broadcast/drop mechanics, not
// about parsing configuration.
type ConfigChange struct {
	Revision int
	Source   string
}

// RunConfig holds the injected dependencies and parameters for a demo run. Nothing
// is read from globals: the logger and counter are supplied by the caller so the
// core is deterministic and testable.
type RunConfig struct {
	// Subscribers is the number of healthy subscribers that keep up with the stream.
	Subscribers int
	// Buffer is the per-subscriber channel buffer. The slow subscriber holds this
	// many events before the rest overflow.
	Buffer int
	// Events is the number of configuration changes to broadcast.
	Events int
	// Source labels the change origin, surfaced as a metric attribute on drops.
	Source string
	// DropCounter observes every dropped event. It may be nil: Add is nil-safe.
	DropCounter *observability.Int64Counter
}

// Result reports what each subscriber observed after a run.
type Result struct {
	// Healthy[i] is the number of events healthy subscriber i received (expected to
	// equal RunConfig.Events).
	Healthy []int
	// SlowReceived is how many events the deliberately-slow subscriber managed to
	// buffer before overflowing (expected to equal RunConfig.Buffer).
	SlowReceived int
	// Dropped is the broadcaster's own drop count for this run.
	Dropped uint64
}

// Run drives the fan-out. It builds a stream.Broadcaster, subscribes Subscribers
// healthy consumers plus one deliberately-slow consumer, and broadcasts Events
// changes. Healthy consumers are drained in lock-step so they never overflow; the
// slow consumer is left undrained until the end, so every change past its buffer is
// dropped. Each drop is bridged to DropCounter via the broadcaster's OnDrop hook.
//
// The overflow is forced by capacity math (Events past Buffer), not timing, so the
// result is deterministic. Run honors ctx cancellation and always closes the
// broadcaster before returning, leaking no goroutines.
func Run(ctx context.Context, cfg RunConfig) (Result, error) {
	if cfg.Subscribers < 1 {
		return Result{}, fmt.Errorf("broadcast-demo: Subscribers must be >= 1, got %d", cfg.Subscribers)
	}
	if cfg.Buffer < 1 {
		return Result{}, fmt.Errorf("broadcast-demo: Buffer must be >= 1, got %d", cfg.Buffer)
	}
	if cfg.Events < 0 {
		return Result{}, fmt.Errorf("broadcast-demo: Events must be >= 0, got %d", cfg.Events)
	}

	counter := cfg.DropCounter
	source := cfg.Source

	b := stream.NewBroadcaster[ConfigChange](
		stream.WithBroadcastBuffer(cfg.Buffer),
		// The hook runs under the broadcaster lock, so it stays cheap: a single
		// counter increment, no logging or blocking work. The human-readable
		// summary is emitted by the caller after the run completes.
		stream.WithBroadcastOnDrop(func() {
			counter.Add(ctx, 1, observability.MetricStringAttribute("source", source))
		}),
	)
	defer b.Close()

	healthy := make([]<-chan ConfigChange, cfg.Subscribers)
	for i := range healthy {
		healthy[i] = b.Subscribe(ctx)
	}
	slow := b.Subscribe(ctx)

	received := make([]int, cfg.Subscribers)
	for e := 1; e <= cfg.Events; e++ {
		if err := ctx.Err(); err != nil {
			return Result{Healthy: received, Dropped: b.DroppedCount()}, err
		}
		b.Broadcast(ConfigChange{Revision: e, Source: source})
		for i, ch := range healthy {
			select {
			case _, ok := <-ch:
				if ok {
					received[i]++
				}
			case <-ctx.Done():
				return Result{Healthy: received, Dropped: b.DroppedCount()}, ctx.Err()
			}
		}
	}

	slowReceived := 0
	for {
		select {
		case _, ok := <-slow:
			if !ok {
				return Result{Healthy: received, SlowReceived: slowReceived, Dropped: b.DroppedCount()}, nil
			}
			slowReceived++
			continue
		default:
		}
		break
	}

	return Result{Healthy: received, SlowReceived: slowReceived, Dropped: b.DroppedCount()}, nil
}
