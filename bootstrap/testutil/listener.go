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

// Listener is a [bootstrap.Listener] that serves on a loopback port chosen at Start. Once quiesced it answers new requests with 503 Service Unavailable; Drain waits for in-flight requests until its context ends. ServeHTTP routes a request in-process without the network.
type Listener struct {
	name string
	mux  *http.ServeMux

	mu       sync.Mutex
	srv      *http.Server
	url      string
	done     chan error
	closing  bool
	stopping bool
}

// NewListener returns a stopped listener whose component name is name.
func NewListener(name string) *Listener {
	return &Listener{name: name, mux: http.NewServeMux()}
}

// Name returns the component name.
func (l *Listener) Name() string { return l.name }

// Handle mounts handler at pattern. Like [http.ServeMux.Handle], it panics on an invalid or conflicting pattern.
func (l *Listener) Handle(pattern string, handler http.Handler) { l.mux.Handle(pattern, handler) }

// ServeHTTP routes r to the mounted handlers, or answers 503 once the listener is quiesced.
func (l *Listener) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	l.mu.Lock()
	closing := l.closing
	l.mu.Unlock()
	if closing {
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	}
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
	l.srv = &http.Server{Handler: l, ReadHeaderTimeout: 5 * time.Second}
	l.url = "http://" + ln.Addr().String()
	l.done = make(chan error, 1)
	go func(srv *http.Server, done chan<- error) { done <- srv.Serve(ln) }(l.srv, l.done)
	return nil
}

// Quiesce makes the listener answer new requests with 503. It is idempotent.
func (l *Listener) Quiesce() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closing = true
	return nil
}

// Drain closes the port and waits for in-flight requests until ctx ends.
func (l *Listener) Drain(ctx context.Context) error {
	srv, _ := l.server()
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

// DrainPhase reports [component.DrainIngress].
func (l *Listener) DrainPhase() component.DrainPhase { return component.DrainIngress }

// Stop shuts the server down, closing connections that remain when ctx ends, and waits for it to stop serving. Stop without Start does nothing.
func (l *Listener) Stop(ctx context.Context) error {
	srv, done := l.server()
	if srv == nil {
		return nil
	}
	l.mu.Lock()
	first := !l.stopping
	l.stopping = true
	l.mu.Unlock()
	if !first {
		return nil
	}
	err := srv.Shutdown(ctx)
	if err != nil {
		err = errors.Join(err, srv.Close())
	}
	if serveErr := <-done; !errors.Is(serveErr, http.ErrServerClosed) {
		err = errors.Join(err, serveErr)
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
