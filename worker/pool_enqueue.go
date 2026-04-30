package worker

import (
	"context"
	"errors"
	"fmt"
)

func (p *Pool[I, O]) enqueue(ctx context.Context, idx int, env taskEnvelope[I, O]) (*TaskHandle[O], error) {
	workerCh := p.workers[idx]

	switch p.cfg.Overflow {
	case Reject:
		select {
		case workerCh <- env:
			return env.handle, nil
		case <-ctx.Done():
			env.handle.Cancel()
			return nil, ctx.Err()
		case <-p.poolCtx.Done():
			env.handle.Cancel()
			return nil, fmt.Errorf("worker: pool %q shutting down", p.cfg.Name)
		default:
			env.handle.Cancel()
			return nil, ErrQueueFull
		}
	case DropOldest:
		for {
			select {
			case workerCh <- env:
				return env.handle, nil
			case <-ctx.Done():
				env.handle.Cancel()
				return nil, ctx.Err()
			case <-p.poolCtx.Done():
				env.handle.Cancel()
				return nil, fmt.Errorf("worker: pool %q shutting down", p.cfg.Name)
			default:
			}

			select {
			case dropped := <-workerCh:
				p.failDroppedTask(dropped)
			case <-ctx.Done():
				env.handle.Cancel()
				return nil, ctx.Err()
			case <-p.poolCtx.Done():
				env.handle.Cancel()
				return nil, fmt.Errorf("worker: pool %q shutting down", p.cfg.Name)
			default:
				env.handle.Cancel()
				return nil, ErrQueueFull
			}
		}
	default:
		select {
		case workerCh <- env:
			return env.handle, nil
		case <-ctx.Done():
			env.handle.Cancel()
			return nil, ctx.Err()
		case <-p.poolCtx.Done():
			env.handle.Cancel()
			return nil, fmt.Errorf("worker: pool %q shutting down", p.cfg.Name)
		}
	}
}

func (p *Pool[I, O]) failDroppedTask(env taskEnvelope[I, O]) {
	env.handle.Cancel()
	var zero O
	droppedErr := ErrTaskDropped
	env.handle.emit(errorEvent[O](droppedErr))
	env.handle.complete(zero, droppedErr)
	if !errors.Is(droppedErr, context.Canceled) {
		p.failCount.Add(1)
	}
}
