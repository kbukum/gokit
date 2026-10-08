package testutil

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/kbukum/gokit/bootstrap"
	"github.com/kbukum/gokit/component"
)

var _ bootstrap.Listener = (*Listener)(nil)

// Listener is a [bootstrap.Listener] that serves on a loopback port chosen at Start. Once quiesced it answers new requests with 503 Service Unavailable. Drain and Stop follow the ingress contract of gokit's server: wait for in-flight requests for most of the deadline, then force-close connections, cancel request contexts and wait for handlers to return. ServeHTTP routes a request in-process without the network.
type Listener struct {
	name string
	mux  *http.ServeMux

	mu       sync.Mutex
	srv      *http.Server
	url      string
	done     chan error
	cancel   context.CancelFunc
	closing  bool
	stopping bool
	active   int           // admitted handlers still running
	drained  chan struct{} // closed once the listener is closing and no handler runs
}

// NewListener returns a stopped listener whose component name is name.
func NewListener(name string) *Listener {
	return &Listener{name: name, mux: http.NewServeMux(), drained: make(chan struct{})}
}

// Name returns the component name.
func (l *Listener) Name() string { return l.name }

// Handle mounts handler at pattern. Like [http.ServeMux.Handle], it panics on an invalid or conflicting pattern.
func (l *Listener) Handle(pattern string, handler http.Handler) { l.mux.Handle(pattern, handler) }

// ServeHTTP routes r to the mounted handlers, or answers 503 once the listener is quiesced.
func (l *Listener) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	l.mu.Lock()
	if l.closing {
		l.mu.Unlock()
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	}
	l.active++
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.active--
		if l.closing && l.active == 0 {
			close(l.drained)
		}
	}()
	l.mux.ServeHTTP(w, r)
}

// URL returns the base URL, such as "http://127.0.0.1:41234", or "" before Start.
func (l *Listener) URL() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.url
}

// Start listens on 127.0.0.1 at a free port and serves in the background.
func (l *Listener) Start(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv != nil {
		return errors.New("testutil: listener " + l.name + " already started")
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	base, cancel := context.WithCancel(context.WithoutCancel(ctx))
	l.cancel = cancel
	l.srv = &http.Server{Handler: l, ReadHeaderTimeout: 5 * time.Second, BaseContext: func(net.Listener) context.Context { return base }}
	l.url = "http://" + ln.Addr().String()
	l.done = make(chan error, 1)
	go func(srv *http.Server, done chan<- error) { done <- srv.Serve(ln) }(l.srv, l.done)
	return nil
}

// Quiesce makes the listener answer new requests with 503. It is idempotent.
func (l *Listener) Quiesce() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.closing {
		l.closing = true
		if l.active == 0 {
			close(l.drained)
		}
	}
	return nil
}

// Drain closes the port, waits for in-flight requests and, if they outlast the deadline, force-closes them; see [Listener.shutdown].
func (l *Listener) Drain(ctx context.Context) error {
	srv, _ := l.server()
	if srv == nil {
		return nil
	}
	return l.shutdown(ctx, srv)
}

// DrainPhase reports [component.DrainIngress].
func (l *Listener) DrainPhase() component.DrainPhase { return component.DrainIngress }

// Stop drains like [Listener.Drain] and waits for the server to stop serving. Stop without Start, or a second Stop, does nothing.
func (l *Listener) Stop(ctx context.Context) error {
	srv, done := l.server()
	l.mu.Lock()
	first := srv != nil && !l.stopping
	if srv != nil {
		l.stopping = true
	}
	l.mu.Unlock()
	if !first {
		return nil
	}
	err := l.shutdown(ctx, srv)
	if serveErr := <-done; !errors.Is(serveErr, http.ErrServerClosed) {
		err = errors.Join(err, serveErr)
	}
	l.cancel()
	return err
}

// shutdown quiesces the listener, stops accepting connections and gives in-flight requests four fifths of ctx's remaining time. If they outlast it, it force-closes connections and cancels request contexts, then waits for handlers to return until ctx ends, so no handler outlives a successful drain. It returns the graceful-shutdown error and any wait failure.
func (l *Listener) shutdown(ctx context.Context, srv *http.Server) error {
	_ = l.Quiesce() // never fails; admission must be closed for drained to close
	graceCtx := ctx
	if deadline, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		graceCtx, cancel = context.WithTimeout(ctx, time.Until(deadline)*4/5)
		defer cancel()
	}
	err := srv.Shutdown(graceCtx)
	if err != nil {
		err = errors.Join(err, srv.Close())
		l.cancel()
	}
	select {
	case <-l.drained:
	case <-ctx.Done():
		err = errors.Join(err, ctx.Err())
	}
	return err
}

// Health reports healthy while the listener is started and not quiesced.
func (l *Listener) Health(context.Context) component.Health {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv == nil || l.closing || l.stopping {
		return component.Health{Name: l.name, Status: component.StatusUnhealthy, Message: "not serving"}
	}
	return component.Health{Name: l.name, Status: component.StatusHealthy}
}

func (l *Listener) server() (srv *http.Server, done chan error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.srv, l.done
}
