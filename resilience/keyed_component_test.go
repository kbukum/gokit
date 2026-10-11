package resilience_test

import (
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/util"
)

func TestKeyedComponentLifecycle(t *testing.T) {
	t.Parallel()
	c := resilience.NewKeyedComponent("admission", resilience.KeyedRateLimiterConfig{MaxKeys: 2, Clock: util.NewFakeClock(time.Unix(0, 0))})
	if c.Name() != "admission" {
		t.Fatal(c.Name())
	}
	for range 2 {
		if c.Health(t.Context()).Status != component.StatusUnhealthy || c.Allow("actor", 1, time.Second).Allowed {
			t.Fatal("stopped limiter admitted work")
		}
		if err := c.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := c.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		if c.Health(t.Context()).Status != component.StatusHealthy || !c.Allow("actor", 1, time.Second).Allowed {
			t.Fatal("started limiter refused work")
		}
		if d := c.Allow("actor", 1, time.Second); d.Allowed || d.RetryAfter <= 0 {
			t.Fatal("exhausted limiter admitted work")
		}
		if err := c.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := c.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
}
