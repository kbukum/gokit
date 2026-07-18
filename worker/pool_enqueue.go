package worker

import (
	"context"
)

func (p *Pool[I, O]) enqueue(ctx context.Context, env taskEnvelope[I, O]) (*TaskHandle[O], error) {
	return p.enqueueTo(ctx, p.queue, env, true)
}

func (p *Pool[I, O]) enqueueAffinity(ctx context.Context, idx int, env taskEnvelope[I, O]) (*TaskHandle[O], error) {
	select {
	case p.affinities[idx] <- env:
		return env.handle, nil
	case <-ctx.Done():
		env.handle.Cancel()
		return nil, ctx.Err()
	case <-p.acceptCtx.Done():
		env.handle.Cancel()
		return nil, p.stoppedError()
	default:
		// Affinity is only a steering hint under supervision. Fall back to the shared pool queue so QueueSize/Overflow semantics remain pool-wide.
		return p.enqueue(ctx, env)
	}
}

func (p *Pool[I, O]) enqueueTo(
	ctx context.Context,
	ch chan taskEnvelope[I, O],
	env taskEnvelope[I, O],
	allowDropOldest bool,
) (*TaskHandle[O], error) {
	switch p.cfg.Overflow {
	case OverflowReject:
		select {
		case ch <- env:
			return env.handle, nil
		case <-ctx.Done():
			env.handle.Cancel()
			return nil, ctx.Err()
		case <-p.acceptCtx.Done():
			env.handle.Cancel()
			return nil, p.stoppedError()
		case <-p.poolCtx.Done():
			env.handle.Cancel()
			return nil, p.stoppedError()
		default:
			env.handle.Cancel()
			return nil, ErrQueueFull
		}
	case OverflowDropOldest:
		if !allowDropOldest || cap(ch) == 0 {
			return p.enqueueBlocking(ctx, ch, env)
		}
		select {
		case ch <- env:
			return env.handle, nil
		case <-ctx.Done():
			env.handle.Cancel()
			return nil, ctx.Err()
		case <-p.acceptCtx.Done():
			env.handle.Cancel()
			return nil, p.stoppedError()
		case <-p.poolCtx.Done():
			env.handle.Cancel()
			return nil, p.stoppedError()
		default:
		}

		select {
		case dropped := <-ch:
			p.failDroppedTask(dropped)
		case <-ctx.Done():
			env.handle.Cancel()
			return nil, ctx.Err()
		case <-p.acceptCtx.Done():
			env.handle.Cancel()
			return nil, p.stoppedError()
		case <-p.poolCtx.Done():
			env.handle.Cancel()
			return nil, p.stoppedError()
		default:
			// A worker drained the queue between probes; enqueue normally and let the pool-wide queue semantics decide the outcome.
		}
		return p.enqueueBlocking(ctx, ch, env)
	default:
		return p.enqueueBlocking(ctx, ch, env)
	}
}

func (p *Pool[I, O]) enqueueBlocking(
	ctx context.Context,
	ch chan taskEnvelope[I, O],
	env taskEnvelope[I, O],
) (*TaskHandle[O], error) {
	select {
	case ch <- env:
		return env.handle, nil
	case <-ctx.Done():
		env.handle.Cancel()
		return nil, ctx.Err()
	case <-p.acceptCtx.Done():
		env.handle.Cancel()
		return nil, p.stoppedError()
	case <-p.poolCtx.Done():
		env.handle.Cancel()
		return nil, p.stoppedError()
	}
}

func (p *Pool[I, O]) failDroppedTask(env taskEnvelope[I, O]) {
	env.handle.Cancel()
	var zero O
	droppedErr := ErrTaskDropped
	env.handle.emit(errorEvent[O](droppedErr))
	env.handle.complete(zero, droppedErr)
	p.taskWg.Done()
	p.failCount.Add(1)
}
