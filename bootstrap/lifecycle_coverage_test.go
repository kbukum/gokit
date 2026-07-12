package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
)

func TestRun_ExitsOnContextCancellation(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	comp := &mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	}
	app.RegisterComponent(comp)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // WaitForSignal returns immediately via ctx.Done

	if err := app.Run(ctx); err != nil {
		t.Fatalf("Run should exit cleanly on canceled context: %v", err)
	}
	if !comp.stopped {
		t.Error("component should be stopped after Run returns")
	}
}

func TestStartupThenShutdown(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	comp := &mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	}
	app.RegisterComponent(comp)

	if err := app.Startup(context.Background()); err != nil {
		t.Fatalf("Startup failed: %v", err)
	}
	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
	if !comp.stopped {
		t.Error("component should be stopped after Shutdown")
	}
}

func TestWaitForSignal_ReturnsNilOnContextCancel(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if sig := app.WaitForSignal(ctx); sig != nil {
		t.Fatalf("expected nil signal on context cancel, got %v", sig)
	}
}
