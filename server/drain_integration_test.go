package server_test

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/kbukum/gokit/bootstrap"
	"github.com/kbukum/gokit/component"
	componenttest "github.com/kbukum/gokit/component/testutil"
	"github.com/kbukum/gokit/config"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/server"
	"github.com/kbukum/gokit/sse"
	ssemetrics "github.com/kbukum/gokit/sse/metrics"
	"github.com/kbukum/gokit/util"
	"github.com/kbukum/gokit/worker"
)

func TestCoordinatedShutdownDrainsBeforeDependencies(t *testing.T) {
	const budget = 500 * time.Millisecond
	const allowance = 250 * time.Millisecond
	app, err := bootstrap.NewApp(&config.ServiceConfig{Name: "shutdown"}, bootstrap.WithGracefulTimeout(budget), bootstrap.WithAdmin(bootstrap.AdminConfig{}))
	if err != nil {
		t.Fatal(err)
	}
	app.Summary.SetWriter(io.Discard)
	bus, err := sse.NewBus(sse.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	streams, err := sse.NewComponent(bus)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := sse.NewHandler(bus, sse.HandlerConfig{
		Logger: logging.NewDefault("shutdown"), Clock: util.SystemClock{},
		WriteTimeout: time.Second, Heartbeat: time.Second,
		Authorize: func(*http.Request) (sse.Access, error) { return sse.Access{Principal: "user", Route: "scope"}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(&server.Config{Host: "127.0.0.1"}, nil)
	srv.Handle("/events", handler)
	slowStarted, slowDone := make(chan struct{}), make(chan struct{})
	srv.Handle("/slow", http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(slowStarted)
		<-r.Context().Done()
		close(slowDone)
	}))
	srv.ApplyDefaults("shutdown", app.Components.HealthAll)
	workerStarted, workerDone := make(chan struct{}), make(chan struct{})
	pool := worker.NewPool(worker.HandlerFunc[int, int](func(ctx context.Context, _ int, emit func(worker.Event[int])) error {
		close(workerStarted)
		<-ctx.Done()
		close(workerDone)
		return ctx.Err()
	}), worker.PoolConfig{Name: "busy", Size: 1, GracePeriod: time.Second})
	defer func() { _ = pool.Stop(context.Background()) }()
	var order []string
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	if err := srv.Instrument(meter.Meter("http")); err != nil {
		t.Fatal(err)
	}
	srv.ApplyMiddleware()
	assertActive := func(ctx context.Context, requests, streams int64) {
		t.Helper()
		var data metricdata.ResourceMetrics
		if err := reader.Collect(ctx, &data); err != nil {
			t.Fatal(err)
		}
		values := make(map[string]int64)
		for _, scope := range data.ScopeMetrics {
			for _, metric := range scope.Metrics {
				switch value := metric.Data.(type) {
				case metricdata.Sum[int64]:
					for _, point := range value.DataPoints {
						values[metric.Name] += point.Value
					}
				case metricdata.Gauge[int64]:
					for _, point := range value.DataPoints {
						values[metric.Name] += point.Value
					}
				}
			}
		}
		for name, want := range map[string]int64{"http_requests_active": requests, "sse.streams.active": streams} {
			if got, ok := values[name]; !ok || got != want {
				t.Errorf("%s: got %d (present=%v), want %d", name, got, ok, want)
			}
		}
	}
	registration, err := ssemetrics.Register(bus, meter.Meter("integration"))
	if err != nil {
		t.Fatal(err)
	}
	clientComponent := &componenttest.Component{ComponentName: "client", StopFunc: func(ctx context.Context) error {
		if stats := bus.Stats(); stats.ActiveStreams != 0 || stats.QueueBytes != 0 {
			t.Errorf("stream resources retained: %+v", stats)
		}
		select {
		case <-workerDone:
		default:
			t.Error("client closed before worker drained")
		}
		select {
		case <-slowDone:
		default:
			t.Error("client closed before HTTP drained")
		}
		if pool.Stats().CancellationCallbacks != 0 {
			t.Error("task callback retained")
		}
		order = append(order, "client")
		assertActive(ctx, 0, 0)
		return registration.Unregister()
	}}
	httpClient := &http.Client{Timeout: 2 * time.Second}
	defer httpClient.CloseIdleConnections()
	telemetry := &componenttest.Component{ComponentName: "telemetry", StopFunc: func(ctx context.Context) error {
		if !slices.Equal(order, []string{"client"}) {
			t.Errorf("telemetry closed before clients: %v", order)
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+app.AdminAddr().String()+"/metrics", http.NoBody)
		if err != nil {
			return err
		}
		response, err := httpClient.Do(request)
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		for _, name := range []string{"go_goroutines", "go_memstats_heap_alloc_bytes"} {
			found := false
			for _, line := range strings.Split(string(body), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 2 && fields[0] == name {
					value, parseErr := strconv.ParseFloat(fields[1], 64)
					if parseErr != nil || value <= 0 {
						t.Errorf("runtime metric %s = %q", name, fields[1])
					}
					found = true
				}
			}
			if !found {
				t.Errorf("runtime metric %s missing", name)
			}
		}
		order = append(order, "telemetry")
		return errors.Join(readErr, closeErr, meter.Shutdown(ctx))
	}}
	for _, c := range []component.Component{clientComponent, streams, pool, server.NewComponent(srv)} {
		if err := app.RegisterComponent(c); err != nil {
			t.Fatal(err)
		}
	}
	if err := app.Components.RegisterInPhase(telemetry, component.PhaseTelemetry); err != nil {
		t.Fatal(err)
	}
	if err := app.Startup(t.Context()); err != nil {
		t.Fatal(err)
	}
	handle, err := pool.Submit(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	<-workerStarted
	streamResponse, err := httpClient.Get("http://" + srv.ListenAddr().String() + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer streamResponse.Body.Close()
	if _, err := bufio.NewReader(streamResponse.Body).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := httpClient.Get("http://" + srv.ListenAddr().String() + "/slow")
		if err == nil {
			_ = response.Body.Close()
		}
	}()
	<-slowStarted
	assertActive(t.Context(), 2, 1)
	adminAddr := app.AdminAddr().String()
	start := time.Now()
	if err := app.Shutdown(t.Context()); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("slow handler force-close outcome: %v", err)
	}
	if elapsed := time.Since(start); elapsed > budget+allowance {
		t.Errorf("shutdown exceeded %v + %v: %v", budget, allowance, elapsed)
	}
	<-requestDone
	<-handle.Done()
	if !slices.Equal(order, []string{"client", "telemetry"}) {
		t.Fatalf("dependency release order: %v", order)
	}
	if response, err := httpClient.Get("http://" + adminAddr + "/metrics"); err == nil {
		_ = response.Body.Close()
		t.Fatal("admin listener still accepting after shutdown")
	}
}
