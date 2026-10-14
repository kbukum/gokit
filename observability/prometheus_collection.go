package observability

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

const collectionTimeout = 2 * time.Second

var errSnapshotUnavailable = errors.New("application metrics snapshot unavailable")

// prometheusCollection gathers one scrape at a time. Prometheus collectors cannot propagate request contexts, so each scrape first collects application metrics through an owned, deadline-bound manual reader. A collection failure rejects the scrape instead of reaching the exporter's global error handler as a partial success; the exporter then translates only that scrape's snapshot.
type prometheusCollection struct {
	reader    *sdkmetric.ManualReader
	gatherers prometheus.Gatherers
	gate      chan struct{}

	mu       sync.Mutex
	active   map[*collectionCall]struct{}
	idle     chan struct{}
	stopped  bool
	snapshot []metricdata.ScopeMetrics
	ready    bool
}

type collectionCall struct{ cancel context.CancelFunc }

// collectionError keeps application error text out of scrape responses while preserving the cause.
type collectionError struct{ cause error }

func (collectionError) Error() string   { return "application metrics collection failed" }
func (e collectionError) Unwrap() error { return e.cause }

func newPrometheusCollection(reader *sdkmetric.ManualReader) *prometheusCollection {
	idle := make(chan struct{})
	close(idle)
	return &prometheusCollection{
		reader: reader, gate: make(chan struct{}, 1), active: make(map[*collectionCall]struct{}), idle: idle,
	}
}

// Gather implements prometheus.Gatherer.
func (p *prometheusCollection) Gather() ([]*dto.MetricFamily, error) {
	ctx, cancel := context.WithTimeout(context.Background(), collectionTimeout)
	defer cancel()
	release, err := p.begin(cancel)
	if err != nil {
		return nil, collectionError{cause: err}
	}
	defer release()
	select {
	case p.gate <- struct{}{}:
		defer func() { <-p.gate }()
	case <-ctx.Done():
		return nil, collectionError{cause: context.Cause(ctx)}
	}
	var data metricdata.ResourceMetrics
	if err := p.reader.Collect(ctx, &data); err != nil {
		return nil, collectionError{cause: err}
	}
	if err := ctx.Err(); err != nil {
		return nil, collectionError{cause: err}
	}
	p.publish(data.ScopeMetrics, true)
	defer p.publish(nil, false)
	return p.gatherers.Gather()
}

// Produce supplies the exporter with the snapshot owned by the gathering scrape.
func (p *prometheusCollection) Produce(context.Context) ([]metricdata.ScopeMetrics, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.ready {
		return nil, errSnapshotUnavailable
	}
	return p.snapshot, nil
}

func (p *prometheusCollection) publish(snapshot []metricdata.ScopeMetrics, ready bool) {
	p.mu.Lock()
	p.snapshot, p.ready = snapshot, ready
	p.mu.Unlock()
}

func (p *prometheusCollection) begin(cancel context.CancelFunc) (func(), error) {
	call := &collectionCall{cancel: cancel}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return nil, sdkmetric.ErrReaderShutdown
	}
	if len(p.active) == 0 {
		p.idle = make(chan struct{})
	}
	p.active[call] = struct{}{}
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		delete(p.active, call)
		if len(p.active) == 0 {
			close(p.idle)
		}
	}, nil
}

func (p *prometheusCollection) stop(ctx context.Context) error {
	p.mu.Lock()
	p.stopped = true
	for call := range p.active {
		call.cancel()
	}
	idle := p.idle
	p.mu.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}
