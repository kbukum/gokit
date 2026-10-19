package jetstream

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/messaging/nats"
)

// fakeConnects is an injected connect that counts attempts per publish timeout and can block or fail them.
type fakeConnects struct {
	mu      sync.Mutex
	calls   map[string]int
	gate    map[string]chan struct{}
	entered map[string]chan struct{}
	fail    map[string]error
}

func newFakeConnects() *fakeConnects {
	return &fakeConnects{calls: map[string]int{}, gate: map[string]chan struct{}{}, entered: map[string]chan struct{}{}, fail: map[string]error{}}
}

// block makes the next attempts for timeout wait until the returned release runs; entered closes when the first one starts.
func (f *fakeConnects) block(timeout string) (entered <-chan struct{}, release func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	gate, in := make(chan struct{}), make(chan struct{})
	f.gate[timeout], f.entered[timeout] = gate, in
	return in, func() { close(gate) }
}

func (f *fakeConnects) connect(_ context.Context, cfg nats.Config) (*Client, error) {
	f.mu.Lock()
	f.calls[cfg.PublishTimeout]++
	gate, in, err := f.gate[cfg.PublishTimeout], f.entered[cfg.PublishTimeout], f.fail[cfg.PublishTimeout]
	delete(f.entered, cfg.PublishTimeout)
	delete(f.fail, cfg.PublishTimeout)
	f.mu.Unlock()
	if in != nil {
		close(in)
	}
	if gate != nil {
		<-gate
	}
	if err != nil {
		return nil, err
	}
	return &Client{cfg: cfg}, nil
}

func (f *fakeConnects) count(timeout string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[timeout]
}

func newTestShared(f *fakeConnects) *sharedClients {
	s := newSharedClients(nats.Config{})
	s.connect = f.connect
	return s
}

func closedClient(c *Client) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func TestSharedClientsShareOneClientPerDuration(t *testing.T) {
	t.Parallel()
	f := newFakeConnects()
	s := newTestShared(f)
	a, releaseA, err := s.acquire(t.Context(), "1s")
	if err != nil {
		t.Fatal(err)
	}
	b, releaseB, err := s.acquire(t.Context(), "1000ms")
	if err != nil {
		t.Fatal(err)
	}
	if a != b || f.count("1s") != 1 {
		t.Fatalf("equal durations got different clients or %d connects", f.count("1s"))
	}
	if err := releaseA.Close(); err != nil || closedClient(a) {
		t.Fatalf("first release closed the shared client (err %v)", err)
	}
	if err := releaseA.Close(); err != nil || closedClient(a) {
		t.Fatalf("a repeated release dropped another holder's claim (err %v)", err)
	}
	if err := releaseB.Close(); err != nil || !closedClient(a) {
		t.Fatalf("last release did not close the client (err %v)", err)
	}
	c, releaseC, err := s.acquire(t.Context(), "1s")
	if err != nil || c == a || f.count("1s") != 2 {
		t.Fatalf("acquire after the last release reused a closed client (err %v)", err)
	}
	_ = releaseC.Close()
}

func TestSharedClientsConnectWithoutBlockingOtherTimeouts(t *testing.T) {
	t.Parallel()
	f := newFakeConnects()
	s := newTestShared(f)
	other, releaseOther, err := s.acquire(t.Context(), "1s")
	if err != nil {
		t.Fatal(err)
	}
	entered, unblock := f.block("2s")
	type result struct {
		client *Client
		rel    *clientRelease
		err    error
	}
	leader, waiter := make(chan result, 1), make(chan result, 1)
	go func() {
		c, r, err := s.acquire(t.Context(), "2s")
		leader <- result{c, r, err}
	}()
	<-entered

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, rel, err := s.acquire(ctx, "3s"); err != nil {
		t.Fatalf("another timeout waited for a slow connect: %v", err)
	} else if err := rel.Close(); err != nil {
		t.Fatal(err)
	}
	if err := releaseOther.Close(); err != nil || !closedClient(other) {
		t.Fatalf("a release waited for a slow connect or failed: %v", err)
	}

	canceled, cancelNow := context.WithCancel(t.Context())
	cancelNow()
	if _, _, err := s.acquire(canceled, "2s"); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting acquire with a canceled context = %v, want context.Canceled", err)
	}
	go func() {
		c, r, err := s.acquire(t.Context(), "2s")
		waiter <- result{c, r, err}
	}()
	unblock()
	first, second := <-leader, <-waiter
	if first.err != nil || second.err != nil || first.client != second.client {
		t.Fatalf("callers of one timeout did not share the attempt: %v, %v", first.err, second.err)
	}
	if got := f.count("2s"); got != 1 {
		t.Fatalf("connects for one timeout = %d, want 1", got)
	}
	_ = first.rel.Close()
	_ = second.rel.Close()
}

func TestSharedClientsDoNotKeepAFailedAttempt(t *testing.T) {
	t.Parallel()
	f := newFakeConnects()
	f.fail["1s"] = errors.New("no route")
	s := newTestShared(f)
	if _, _, err := s.acquire(t.Context(), "1s"); err == nil {
		t.Fatal("acquire hid a failed connect")
	}
	c, rel, err := s.acquire(t.Context(), "1s")
	if err != nil || c == nil || f.count("1s") != 2 {
		t.Fatalf("acquire after a failure = %v after %d connects, want a fresh attempt", err, f.count("1s"))
	}
	_ = rel.Close()
}

func TestSharedClientsRejectInvalidTimeouts(t *testing.T) {
	t.Parallel()
	f := newFakeConnects()
	s := newTestShared(f)
	for _, timeout := range []string{"", "soon", "0s", "-1s"} {
		if _, _, err := s.acquire(t.Context(), timeout); err == nil {
			t.Errorf("acquire accepted timeout %q", timeout)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("invalid timeouts reached connect: %v", f.calls)
	}
}
