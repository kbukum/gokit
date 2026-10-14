package observability_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/observability"
)

func TestPrometheusCollectionFailureRejectsScrapeAndRecovers(t *testing.T) {
	t.Parallel()
	exporter, err := observability.NewPrometheusComponent()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := exporter.Stop(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	var fail atomic.Bool
	fail.Store(true)
	_, err = exporter.Meter("test").Int64ObservableGauge("retry_gauge", metric.WithInt64Callback(func(_ context.Context, observer metric.Int64Observer) error {
		observer.Observe(7)
		if fail.Load() {
			return errors.New("application collection failed")
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, failing := range []bool{true, false} {
		fail.Store(failing)
		response := httptest.NewRecorder()
		exporter.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
		if failing {
			if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "application collection failed") {
				t.Fatalf("failed callback returned %d or exposed its error", response.Code)
			}
		} else if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "retry_gauge{") {
			t.Fatalf("collection failed to recover: %d", response.Code)
		}
	}
}

func TestPrometheusConcurrentScrapesIsolateSnapshots(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		exporter, err := observability.NewPrometheusComponent()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := exporter.Stop(context.WithoutCancel(t.Context())); err != nil {
				t.Error(err)
			}
		})
		entered, release := make(chan int64, 2), make(chan struct{})
		var calls atomic.Int64
		_, err = exporter.Meter("test").Int64ObservableGauge("isolated_gauge", metric.WithInt64Callback(func(ctx context.Context, observer metric.Int64Observer) error {
			value := calls.Add(1)
			entered <- value
			select {
			case <-release:
				observer.Observe(value)
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}))
		if err != nil {
			t.Fatal(err)
		}
		first, second := httptest.NewRecorder(), httptest.NewRecorder()
		firstDone, secondDone := make(chan struct{}), make(chan struct{})
		go func() {
			exporter.Handler().ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
			close(firstDone)
		}()
		if value := <-entered; value != 1 {
			t.Fatalf("first callback: %d", value)
		}
		go func() {
			exporter.Handler().ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
			close(secondDone)
		}()
		synctest.Wait()
		release <- struct{}{}
		<-firstDone
		if value := <-entered; value != 2 {
			t.Fatalf("second callback: %d", value)
		}
		release <- struct{}{}
		<-secondDone
		for _, tc := range []struct {
			response *httptest.ResponseRecorder
			suffix   string
		}{{first, "} 1"}, {second, "} 2"}} {
			found := false
			for line := range strings.SplitSeq(tc.response.Body.String(), "\n") {
				if strings.HasPrefix(line, "isolated_gauge{") && strings.HasSuffix(line, tc.suffix) {
					found = true
				}
			}
			if tc.response.Code != http.StatusOK || !found {
				t.Fatalf("scrape snapshot mixed or missing: %d", tc.response.Code)
			}
		}
	})
}

func TestPrometheusCollectionDeadlineCancelsCallback(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		exporter, err := observability.NewPrometheusComponent()
		if err != nil {
			t.Fatal(err)
		}
		if err := exporter.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		callbackDone := make(chan error, 1)
		release := make(chan struct{})
		defer close(release)
		_, err = exporter.Meter("test").Int64ObservableGauge("bounded_gauge", metric.WithInt64Callback(func(ctx context.Context, _ metric.Int64Observer) error {
			select {
			case <-ctx.Done():
			case <-release:
			}
			callbackDone <- ctx.Err()
			return ctx.Err()
		}))
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		served := make(chan struct{})
		go func() {
			exporter.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
			close(served)
		}()
		select {
		case err := <-callbackDone:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("collection deadline: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("scrape timeout did not cancel callback")
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			if err := exporter.Stop(ctx); err != nil {
				t.Logf("pending collection: %v", err)
			}
			return
		}
		<-served
		if err := exporter.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPrometheusShutdownRetainsUncooperativeCollection(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		exporter, err := observability.NewPrometheusComponent()
		if err != nil {
			t.Fatal(err)
		}
		if err := exporter.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
		_, err = exporter.Meter("test").Int64ObservableGauge("pending_gauge", metric.WithInt64Callback(func(context.Context, metric.Int64Observer) error {
			close(entered)
			<-release
			close(finished)
			return nil
		}))
		if err != nil {
			t.Fatal(err)
		}
		served := make(chan struct{})
		go func() {
			exporter.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
			close(served)
		}()
		<-entered
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		stopErr := exporter.Stop(ctx)
		close(release)
		<-finished
		<-served
		if !errors.Is(stopErr, context.DeadlineExceeded) {
			t.Errorf("shutdown reported completion with an active callback: %v", stopErr)
		}
		if err := exporter.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPrometheusShutdownCancelsAndDrainsCollection(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		exporter, err := observability.NewPrometheusComponent()
		if err != nil {
			t.Fatal(err)
		}
		if err := exporter.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		entered := make(chan struct{})
		canceled := make(chan error, 1)
		_, err = exporter.Meter("test").Int64ObservableGauge("cancel_gauge", metric.WithInt64Callback(func(ctx context.Context, _ metric.Int64Observer) error {
			close(entered)
			<-ctx.Done()
			canceled <- ctx.Err()
			return ctx.Err()
		}))
		if err != nil {
			t.Fatal(err)
		}
		served := make(chan struct{})
		go func() {
			exporter.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
			close(served)
		}()
		<-entered
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		if err := exporter.Stop(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-canceled; !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown did not cancel collection: %v", err)
		}
		<-served
		response := httptest.NewRecorder()
		exporter.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("stopped metrics served a scrape: %d", response.Code)
		}
		counter, err := exporter.Meter("stopped").Int64Counter("after_shutdown")
		if err != nil {
			t.Fatal(err)
		}
		counter.Add(t.Context(), 1)
	})
}

func TestPrometheusOwnsApplicationAndRuntimeMetrics(t *testing.T) {
	t.Parallel()
	first, err := observability.NewPrometheusComponent()
	if err != nil {
		t.Fatal(err)
	}
	second, err := observability.NewPrometheusComponent()
	if err != nil {
		t.Fatal(err)
	}
	for _, exporter := range []*observability.PrometheusComponent{first, second} {
		if err := exporter.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := exporter.Stop(context.WithoutCancel(t.Context())); err != nil {
				t.Error(err)
			}
		})
	}
	counter, err := first.Meter("test").Int64Counter("owned_requests")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(t.Context(), 3, metric.WithAttributes(attribute.String("route", "fixed")))
	for _, tc := range []struct {
		exporter *observability.PrometheusComponent
		present  bool
	}{{first, true}, {second, false}} {
		response := httptest.NewRecorder()
		tc.exporter.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "go_goroutines") ||
			strings.Contains(response.Body.String(), "owned_requests_total{") != tc.present {
			t.Fatalf("instance-owned metrics missing or shared: %d", response.Code)
		}
	}
	if first.ShutdownPhase() != component.PhaseTelemetry {
		t.Fatal("metrics must drain after application consumers")
	}
}

func TestPrometheusLifecycle(t *testing.T) {
	t.Parallel()
	exporter, err := observability.NewPrometheusComponent()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := exporter.Start(ctx); err == nil {
		t.Fatal("canceled startup succeeded")
	}
	if health := exporter.Health(t.Context()); health.Status != component.StatusDegraded {
		t.Fatal("unstarted metrics reported healthy")
	}
	if err := exporter.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := exporter.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if health := exporter.Health(t.Context()); health.Status != component.StatusHealthy {
		t.Fatal("running metrics reported unavailable")
	}
	if exporter.Name() != "metrics" {
		t.Fatal("unstable component identity")
	}
	if err := exporter.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := exporter.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := exporter.Start(t.Context()); err == nil {
		t.Fatal("stopped metrics restarted")
	}
	if health := exporter.Health(t.Context()); health.Status != component.StatusDegraded {
		t.Fatal("stopped metrics reported healthy")
	}
}
