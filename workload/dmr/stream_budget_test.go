package dmr

import (
	"testing"
	"time"
)

func TestPullRetainsConfiguredLongStreamBudget(t *testing.T) {
	cfg := Config{PullTimeout: 30 * time.Minute}
	cfg.ApplyDefaults()
	c, err := newClient(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	stream := c.http.GetConfig().Stream
	if stream.TotalTimeout != cfg.PullTimeout || stream.FirstProgressTimeout != cfg.PullTimeout || stream.IdleTimeout != cfg.PullTimeout {
		t.Fatalf("model pull inherited inference-scale budgets: %+v", stream)
	}
}
