package worker

import (
	"context"

	"github.com/kbukum/gokit/component"
)

// Name identifies the pool when registered in a component registry.
func (p *Pool[I, O]) Name() string {
	if p.cfg.Name == "" {
		return "worker"
	}
	return p.cfg.Name
}

// Start starts workers under registry ownership. Direct callers can also start lazily through the first Submit.
func (p *Pool[I, O]) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.acceptCtx.Err() != nil {
		return p.stoppedError()
	}
	p.startLocked()
	return nil
}

func (p *Pool[I, O]) startLocked() {
	if p.started {
		return
	}
	p.started = true
	for i := range p.cfg.Size {
		p.wg.Add(1)
		go p.runWorker(i)
	}
	if p.supervisor != nil {
		p.supWg.Add(1)
		go func() {
			defer p.supWg.Done()
			p.supervisor.run(p.poolCtx)
		}()
	}
}

// Quiesce stops new submissions without canceling accepted work.
func (p *Pool[I, O]) Quiesce() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.acceptCancel()
	return nil
}

func (p *Pool[I, O]) DrainPhase() component.DrainPhase { return component.DrainWorkers }
func (p *Pool[I, O]) Drain(ctx context.Context) error  { return p.Stop(ctx) }

func (p *Pool[I, O]) Health(context.Context) component.Health {
	status := component.StatusHealthy
	if p.acceptCtx.Err() != nil {
		status = component.StatusUnhealthy
	}
	return component.Health{Name: p.Name(), Status: status}
}
