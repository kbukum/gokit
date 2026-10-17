package client

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"connectrpc.com/connect"

	"github.com/kbukum/gokit/component"
)

type idleCounter struct {
	http.RoundTripper
	closed int
}

func (c *idleCounter) CloseIdleConnections() { c.closed++ }

func TestPeerRequiresANameAndClient(t *testing.T) {
	if _, err := NewPeer("", &http.Client{}); err == nil {
		t.Fatal("unnamed peer accepted")
	}
	if _, err := NewPeer("control", nil); err == nil {
		t.Fatal("peer without a client accepted")
	}
}

func TestPeerReportsObservedAvailabilityAndReleasesConnections(t *testing.T) {
	idle := &idleCounter{}
	hc := &http.Client{Transport: idle}
	p, err := NewPeer("peer-control", hc)
	if err != nil {
		t.Fatal(err)
	}
	var _ component.Component = p
	if p.Name() != "peer-control" || p.HTTPClient() != hc {
		t.Fatalf("peer = %q, %p", p.Name(), p.HTTPClient())
	}
	if err := p.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if h := p.Health(t.Context()); h.Status != component.StatusHealthy || h.Name != "peer-control" {
		t.Fatalf("unobserved health = %+v", h)
	}
	p.Availability().Observe(context.Background(), connect.NewError(connect.CodeInternal, &transportError{cause: errors.New("refused")}))
	if h := p.Health(t.Context()); h.Status != component.StatusDegraded {
		t.Fatalf("health after an outage = %+v", h)
	}
	if err := p.Stop(t.Context()); err != nil || idle.closed != 1 {
		t.Fatalf("stop = %v, idle closes %d", err, idle.closed)
	}
}
