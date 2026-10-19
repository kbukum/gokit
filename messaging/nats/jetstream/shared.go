package jetstream

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/messaging/nats"
)

// sharedClients lends one [Client] per publish timeout to the producers and consumers one [Register] call builds, so a service with many topics holds one connection rather than one per topic. The last holder to close releases it.
//
// Connecting happens outside the lock: callers that need the same timeout wait for the one connection attempt in flight, while other timeouts and releases proceed.
type sharedClients struct {
	cfg     nats.Config
	connect func(context.Context, nats.Config) (*Client, error)
	mu      sync.Mutex
	live    map[time.Duration]*clientLease
}

// clientLease is the shared client for one timeout. ready closes when its connection attempt ends; only leases whose attempt succeeded stay in sharedClients.live after that.
type clientLease struct {
	ready  chan struct{}
	client *Client
	refs   int
}

func newSharedClients(cfg nats.Config) *sharedClients {
	return &sharedClients{cfg: cfg, connect: New, live: make(map[time.Duration]*clientLease)}
}

// acquire returns a live client for timeout and a release that the holder must call exactly once. Timeouts are compared by duration, so "1s" and "1000ms" share a client. A client whose connection closed for good is replaced; its remaining holders keep their lease until they close. A failed connection attempt is not shared: each waiting caller then makes its own attempt under its own context.
func (s *sharedClients) acquire(ctx context.Context, timeout string) (*Client, *clientRelease, error) {
	key, err := time.ParseDuration(timeout)
	if err != nil || key <= 0 {
		return nil, nil, fmt.Errorf("jetstream: request timeout %q must be a positive duration", timeout)
	}
	for {
		s.mu.Lock()
		lease := s.live[key]
		if lease == nil || lease.isDead() {
			return s.open(ctx, key)
		}
		select {
		case <-lease.ready:
			lease.refs++
			s.mu.Unlock()
			return lease.client, &clientRelease{shared: s, key: key, lease: lease}, nil
		default:
		}
		s.mu.Unlock()
		select {
		case <-lease.ready:
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}
}

// open publishes a connecting lease for key, connects without holding the lock and settles the lease. It is called with s.mu held and releases it.
func (s *sharedClients) open(ctx context.Context, key time.Duration) (*Client, *clientRelease, error) {
	lease := &clientLease{ready: make(chan struct{}), refs: 1}
	s.live[key] = lease
	s.mu.Unlock()

	cfg := s.cfg
	cfg.PublishTimeout = key.String()
	client, err := s.connect(ctx, cfg)

	s.mu.Lock()
	defer s.mu.Unlock()
	lease.client = client
	close(lease.ready)
	if err != nil {
		if s.live[key] == lease {
			delete(s.live, key)
		}
		return nil, nil, err
	}
	return client, &clientRelease{shared: s, key: key, lease: lease}, nil
}

// isDead reports that a settled lease's connection closed for good. A lease still connecting is not dead.
func (l *clientLease) isDead() bool {
	select {
	case <-l.ready:
		return l.client.dead()
	default:
		return false
	}
}

// clientRelease is one holder's claim on a shared client.
type clientRelease struct {
	shared *sharedClients
	key    time.Duration
	lease  *clientLease
	once   sync.Once
}

// Close drops the claim and closes the client when no holder remains.
func (r *clientRelease) Close() error {
	var last bool
	r.once.Do(func() {
		s := r.shared
		s.mu.Lock()
		r.lease.refs--
		last = r.lease.refs == 0
		if last && s.live[r.key] == r.lease {
			delete(s.live, r.key)
		}
		s.mu.Unlock()
	})
	if !last {
		return nil
	}
	return r.lease.client.Close()
}
