package testutil

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/testutil"
)

func TestComponent_Interfaces(t *testing.T) {
	comp := NewComponent()
	var _ component.Component = comp
	var _ testutil.TestComponent = comp
}

func TestComponent_Lifecycle(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()

	if comp.BaseURL() != "" {
		t.Error("BaseURL() should be empty before Start")
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if comp.BaseURL() == "" {
		t.Error("BaseURL() should not be empty after Start")
	}

	health := comp.Health(ctx)
	if health.Status != component.StatusHealthy {
		t.Errorf("Health = %q, want %q", health.Status, component.StatusHealthy)
	}

	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

func TestComponent_ServeRoutes(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()

	comp.GinEngine().GET("/hello", func(c *gin.Context) {
		c.String(http.StatusOK, "world")
	})

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer comp.Stop(ctx)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, comp.BaseURL()+"/hello", http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /hello failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "world" {
		t.Errorf("body = %q, want %q", string(body), "world")
	}
}

func TestComponent_Reset(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()

	comp.GinEngine().GET("/before", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer comp.Stop(ctx)

	// Verify route works
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, comp.BaseURL()+"/before", http.NoBody)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("before Reset: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Reset preserves mounted routes and the current origin.
	origin := comp.BaseURL()
	if err := comp.Reset(ctx); err != nil {
		t.Fatalf("Reset() failed: %v", err)
	}

	if comp.BaseURL() != origin {
		t.Fatal("reset changed the owned origin")
	}
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, comp.BaseURL()+"/before", http.NoBody)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("after Reset: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestStopAllowsCanceledHandlerToReadHealth(t *testing.T) {
	t.Parallel()
	c := NewComponent()
	exited := make(chan struct{})
	c.Handle("/stream", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(exited)
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		c.Health(context.Background())
		c.BaseURL()
	}))
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL()+"/stream", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	stopCtx, stopCancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer stopCancel()
	stopped := make(chan error, 1)
	go func() { stopped <- c.Stop(stopCtx) }()
	select {
	case err := <-stopped:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("drain failure: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("teardown deadlocked a cancellation-cooperative handler")
	}
	select {
	case <-exited:
	case <-ctx.Done():
		t.Fatal("handler retained")
	}
}
