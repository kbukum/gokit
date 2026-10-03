package testutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/component/testutil"
)

func TestComponentLifecycleHooks(t *testing.T) {
	t.Parallel()
	fixture := &testutil.Component{ComponentName: "client"}
	if fixture.Name() != "client" || fixture.Start(t.Context()) != nil || fixture.Stop(t.Context()) != nil || fixture.Health(t.Context()).Status != component.StatusHealthy {
		t.Fatal("default lifecycle failed")
	}
	want := errors.New("injected failure")
	fixture.StartFunc = func(context.Context) error { return want }
	fixture.StopFunc = func(context.Context) error { return want }
	fixture.HealthFunc = func(context.Context) component.Health { return component.Health{Status: component.StatusUnhealthy} }
	if !errors.Is(fixture.Start(t.Context()), want) || !errors.Is(fixture.Stop(t.Context()), want) || fixture.Health(t.Context()).Status != component.StatusUnhealthy {
		t.Fatal("lifecycle hooks did not preserve outcomes")
	}
}
