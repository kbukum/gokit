package testutil

import (
	"context"
	"io"
	"net/http"
	"testing"

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

	// Reset clears routes
	if err := comp.Reset(ctx); err != nil {
		t.Fatalf("Reset() failed: %v", err)
	}

	// Old route should be gone (404)
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, comp.BaseURL()+"/before", http.NoBody)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("after Reset: status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
