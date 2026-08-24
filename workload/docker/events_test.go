package docker

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/events"

	"github.com/kbukum/gokit/workload"
)

func TestMapDockerEventNormalizesKnownActions(t *testing.T) {
	t.Parallel()

	cases := map[events.Action]string{
		events.ActionStart:   "start",
		events.ActionStop:    "stop",
		events.ActionDie:     "die",
		events.ActionKill:    "kill",
		events.ActionRestart: "restart",
		events.ActionOOM:     "oom",
		events.ActionCreate:  "create",
		events.ActionDestroy: "destroy",
		events.ActionPause:   "pause",
		events.ActionUnPause: "unpause",
		"health_status":      "health_status",
	}
	for action, want := range cases {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()
			if got := mapDockerEvent(action); got != want {
				t.Fatalf("mapDockerEvent(%q) = %q, want %q", action, got, want)
			}
		})
	}
}

func TestWatchEventsStreamsWorkloadEvents(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, func(req *http.Request) (int, string) {
		if dockerPath(req.URL.Path) != "/events" {
			return http.StatusNotFound, `{}`
		}
		if !strings.Contains(req.URL.Query().Get("filters"), "container") {
			return http.StatusBadRequest, `{"message":"missing filter"}`
		}
		return http.StatusOK, `{"Type":"container","Action":"start","Actor":{"ID":"abc","Attributes":{"name":"worker"}},"time":1710000000}` + "\n"
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := manager.WatchEvents(ctx, workload.ListFilter{Name: "worker", Labels: map[string]string{"team": "platform"}})
	if err != nil {
		t.Fatalf("watch events: %v", err)
	}
	evt, ok := <-ch
	if !ok {
		t.Fatal("event channel closed before first event")
	}
	if evt.ID != "abc" || evt.Name != "worker" || evt.Event != "start" || evt.Timestamp.IsZero() || evt.Message != "start" {
		t.Fatalf("event = %#v", evt)
	}
}

func TestWatchEventsClosesOnTransportError(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, func(*http.Request) (int, string) {
		return http.StatusInternalServerError, `{"message":"boom"}`
	})
	ch, err := manager.WatchEvents(context.Background(), workload.ListFilter{})
	if err != nil {
		t.Fatalf("watch events: %v", err)
	}
	if _, ok := <-ch; ok {
		t.Fatal("event channel should close on stream error")
	}
}

func TestWatchEventsClosesOnContextCancelWhileStreamIsOpen(t *testing.T) {
	t.Parallel()

	manager := newTestManagerHTTP(t, func(req *http.Request) (*http.Response, error) {
		if dockerPath(req.URL.Path) != "/events" {
			return &http.Response{StatusCode: http.StatusNotFound, Status: http.StatusText(http.StatusNotFound), Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     http.StatusText(http.StatusOK),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       contextReadCloser{ctx: req.Context()},
			Request:    req,
		}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := manager.WatchEvents(ctx, workload.ListFilter{})
	if err != nil {
		t.Fatalf("watch events: %v", err)
	}
	cancel()
	if _, ok := <-ch; ok {
		t.Fatal("event channel should close after context cancellation")
	}
}

func TestWatchImageEventsStreamsImageEvents(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, func(req *http.Request) (int, string) {
		if dockerPath(req.URL.Path) != "/events" {
			return http.StatusNotFound, `{}`
		}
		if !strings.Contains(req.URL.Query().Get("filters"), "pull") {
			return http.StatusBadRequest, `{"message":"missing action"}`
		}
		return http.StatusOK, `{"Type":"image","Action":"pull","Actor":{"ID":"sha256:abc","Attributes":{"name":"repo/app:1"}},"timeNano":1710000000123456789}` + "\n"
	})

	ch, err := manager.WatchImageEvents(context.Background(), workload.ImageEventFilter{Actions: []string{"pull"}})
	if err != nil {
		t.Fatalf("watch image events: %v", err)
	}
	evt, ok := <-ch
	if !ok {
		t.Fatal("image event channel closed before first event")
	}
	if evt.Action != "pull" || evt.ImageID != "sha256:abc" || evt.ImageRef != "repo/app:1" || evt.Timestamp.Nanosecond() == 0 {
		t.Fatalf("image event = %#v", evt)
	}
}

func TestWatchImageEventsClosesOnErrorAndCancel(t *testing.T) {
	t.Parallel()

	t.Run("daemon error", func(t *testing.T) {
		t.Parallel()

		manager := newTestManager(t, func(*http.Request) (int, string) {
			return http.StatusInternalServerError, `{"message":"events failed"}`
		})
		ch, err := manager.WatchImageEvents(context.Background(), workload.ImageEventFilter{})
		if err != nil {
			t.Fatalf("watch image events: %v", err)
		}
		if _, ok := <-ch; ok {
			t.Fatal("image event channel should close on stream error")
		}
	})

	t.Run("context cancel", func(t *testing.T) {
		t.Parallel()

		manager := newTestManagerHTTP(t, func(req *http.Request) (*http.Response, error) {
			if dockerPath(req.URL.Path) != "/events" {
				return &http.Response{StatusCode: http.StatusNotFound, Status: http.StatusText(http.StatusNotFound), Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     http.StatusText(http.StatusOK),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       contextReadCloser{ctx: req.Context()},
				Request:    req,
			}, nil
		})

		ctx, cancel := context.WithCancel(context.Background())
		ch, err := manager.WatchImageEvents(ctx, workload.ImageEventFilter{})
		if err != nil {
			t.Fatalf("watch image events: %v", err)
		}
		cancel()
		if _, ok := <-ch; ok {
			t.Fatal("image event channel should close after context cancellation")
		}
	})
}

func TestImageEventHelpersPreferSpecificFields(t *testing.T) {
	t.Parallel()

	byName := events.Message{Actor: events.Actor{ID: "id", Attributes: map[string]string{"name": "name", "image": "image"}}}
	if got := imageRefFromEvent(byName); got != "name" {
		t.Fatalf("name ref = %q", got)
	}
	byImage := events.Message{Actor: events.Actor{ID: "id", Attributes: map[string]string{"image": "image"}}}
	if got := imageRefFromEvent(byImage); got != "image" {
		t.Fatalf("image ref = %q", got)
	}
	byID := events.Message{Actor: events.Actor{ID: "id", Attributes: map[string]string{}}}
	if got := imageRefFromEvent(byID); got != "id" {
		t.Fatalf("id ref = %q", got)
	}
	if got := eventTimestamp(events.Message{TimeNano: 123}); !got.Equal(time.Unix(0, 123)) {
		t.Fatalf("nanos timestamp = %v", got)
	}
	if got := eventTimestamp(events.Message{Time: 456}); !got.Equal(time.Unix(456, 0)) {
		t.Fatalf("seconds timestamp = %v", got)
	}
}

func TestStaticAndFunctionResolvers(t *testing.T) {
	t.Parallel()

	if host, err := (StaticResolver{Host: "tcp://docker"}).ResolveHost(context.Background()); err != nil || host != "tcp://docker" {
		t.Fatalf("static resolver host=%q err=%v", host, err)
	}
	wantErr := errors.New("resolve failed")
	resolver := HostResolverFunc(func(context.Context) (string, error) { return "", wantErr })
	if _, err := resolver.ResolveHost(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("resolver error = %v", err)
	}
}

func TestManagerPoolResolverErrorAndDefaultFallback(t *testing.T) {
	t.Parallel()

	cfg := &Config{Host: "http://docker.example"}
	pool, err := NewManagerPool(cfg, nil, nil, WithHostResolver(HostResolverFunc(func(context.Context) (string, error) {
		return "", errors.New("lookup failed")
	})))
	if err != nil {
		t.Fatalf("new manager pool: %v", err)
	}
	defer func() {
		if err := pool.Close(); err != nil {
			t.Fatalf("close pool: %v", err)
		}
	}()

	if _, err := pool.For(context.Background()); err == nil || !strings.Contains(err.Error(), "resolve host") {
		t.Fatalf("expected resolver error, got %v", err)
	}

	fallback, err := NewManagerPool(&Config{Host: "http://docker.example"}, nil, nil, WithHostResolver(HostResolverFunc(func(context.Context) (string, error) {
		return "", nil
	})))
	if err != nil {
		t.Fatalf("new fallback pool: %v", err)
	}
	defer func() {
		if err := fallback.Close(); err != nil {
			t.Fatalf("close fallback pool: %v", err)
		}
	}()
	if got, err := fallback.For(context.Background()); err != nil || got != fallback.Default() {
		t.Fatalf("default fallback manager=%p default=%p err=%v", got, fallback.Default(), err)
	}
}
