package triton

import (
	"context"
	"net/http"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/httpclient"
)

// Start marks the adapter ready.
func (p *Provider) Start(_ context.Context) error {
	p.lifecycle.MarkReady()
	return nil
}

// Stop closes idle HTTP connections.
func (p *Provider) Stop(ctx context.Context) error {
	p.lifecycle.MarkStopped()
	return p.client.Close(ctx)
}

// Health reports component health using Triton's readiness probe.
func (p *Provider) Health(ctx context.Context) component.Health {
	if !p.lifecycle.Ready() {
		return component.Health{Name: p.Name(), Status: component.StatusDegraded, Message: "not started"}
	}
	if err := p.healthCheck(ctx); err != nil {
		return component.Health{Name: p.Name(), Status: component.StatusUnhealthy, Message: err.Error()}
	}
	msg := "ready"
	if last := p.lifecycle.LastCall(); !last.IsZero() {
		msg = "last_call=" + last.UTC().Format("2006-01-02T15:04:05Z")
	}
	return component.Health{Name: p.Name(), Status: component.StatusHealthy, Message: msg}
}

// healthCheck probes /v2/health/ready.
func (p *Provider) healthCheck(ctx context.Context) error {
	ctx, span := startSpan(ctx, "health")
	defer span.End()

	_, err := p.do(ctx, httpclient.Request{Method: http.MethodGet, Path: "/v2/health/ready"})
	if err != nil {
		span.RecordError(err)
		span.SetError(err.Error())
	}
	return err
}
