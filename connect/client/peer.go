package client

import (
	"context"
	"errors"
	"net/http"

	"github.com/kbukum/gokit/component"
)

// Peer owns one peer's HTTP client and the [Availability] its calls report, under a component lifecycle. Health is
// what real calls last observed, so a peer is never probed; Stop releases idle connections once the callers have
// drained. Build every Connect client for the peer from HTTPClient with Availability installed.
type Peer struct {
	name         string
	http         *http.Client
	availability *Availability
}

var _ component.Component = (*Peer)(nil)

// NewPeer returns the lifecycle owner of hc, reporting availability under name.
func NewPeer(name string, hc *http.Client) (*Peer, error) {
	if name == "" || hc == nil {
		return nil, errors.New("connect client: a peer needs a name and an HTTP client")
	}
	return &Peer{name: name, http: hc, availability: NewAvailability(name)}, nil
}

// HTTPClient is the peer's client.
func (p *Peer) HTTPClient() *http.Client { return p.http }

// Availability is the record every client for this peer must report into.
func (p *Peer) Availability() *Availability { return p.availability }

func (p *Peer) Name() string                                { return p.name }
func (*Peer) Start(context.Context) error                   { return nil }
func (p *Peer) Health(ctx context.Context) component.Health { return p.availability.Health(ctx) }

func (p *Peer) Stop(context.Context) error {
	p.http.CloseIdleConnections()
	return nil
}
