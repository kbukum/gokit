package redis

import (
	"context"
	"strings"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
)

func TestComponentStartsStopsAndReportsHealth(t *testing.T) {
	t.Parallel()

	_, mini := newTestClient(t)
	ctx := context.Background()
	cmp := NewComponent(Config{Enabled: true, Addr: mini.Addr()}, logging.NewDefault("test"))
	if cmp.Name() != "redis" {
		t.Fatalf("Name = %q", cmp.Name())
	}
	if cmp.Client() != nil {
		t.Fatal("Client before Start is not nil")
	}
	if health := cmp.Health(ctx); health.Status != component.StatusUnhealthy {
		t.Fatalf("Health before Start = %+v", health)
	}
	if err := cmp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if cmp.Client() == nil {
		t.Fatal("Client after Start is nil")
	}
	if health := cmp.Health(ctx); health.Status != component.StatusHealthy {
		t.Fatalf("Health after Start = %+v", health)
	}
	if desc := cmp.Describe(); desc.Name != "Redis" || desc.Type != "redis" || !strings.Contains(desc.Details, mini.Addr()) {
		t.Fatalf("Describe = %+v", desc)
	}
	if err := cmp.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if health := cmp.Health(ctx); health.Status != component.StatusUnhealthy {
		t.Fatalf("Health after Stop = %+v", health)
	}
}

func TestComponentSkipsEmptyAddress(t *testing.T) {
	t.Parallel()

	cmp := NewComponent(Config{Enabled: true}, logging.NewDefault("test"))
	if err := cmp.Start(context.Background()); err != nil {
		t.Fatalf("Start empty addr: %v", err)
	}
	if cmp.Client() != nil {
		t.Fatal("empty addr component constructed a client")
	}
	if err := cmp.Stop(context.Background()); err != nil {
		t.Fatalf("Stop with nil client: %v", err)
	}
}

func TestComponentStartAndHealthFailures(t *testing.T) {
	t.Parallel()

	cmp := NewComponent(Config{Enabled: true, Addr: "127.0.0.1:1", DialTimeout: "1ms", ReadTimeout: "1ms", WriteTimeout: "1ms"}, logging.NewDefault("test"))
	if err := cmp.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "ping") {
		t.Fatalf("Start unreachable error = %v", err)
	}

	client, _ := newTestClient(t)
	cmp.client = client
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if health := cmp.Health(context.Background()); health.Status != component.StatusUnhealthy || !strings.Contains(health.Message, "ping") {
		t.Fatalf("Health closed client = %+v", health)
	}
}
