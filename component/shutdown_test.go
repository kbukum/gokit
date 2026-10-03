package component

import (
	"context"
	"reflect"
	"testing"
)

func TestShutdownClosesResourcesBeforeTelemetryAndAdmin(t *testing.T) {
	t.Parallel()
	var order []string
	registry := NewRegistry()
	for _, item := range []struct {
		name  string
		phase ShutdownPhase
	}{{"admin", PhaseAdmin}, {"client", PhaseResources}, {"telemetry", PhaseTelemetry}, {"database", PhaseResources}} {
		if err := registry.RegisterInPhase(&mockComponent{name: item.name, stopOrder: &order}, item.phase); err != nil {
			t.Fatal(err)
		}
	}
	if err := registry.StartAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := registry.Shutdown(t.Context(), func(context.Context) error {
		order = append(order, "container")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"database", "client", "container", "telemetry", "admin"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("shutdown order %v, want %v", order, want)
	}
}
