package metric_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/bench"
	"github.com/kbukum/gokit/bench/metric"
)

// staticContextMetric is a deterministic ContextMetric double for seam tests.
type staticContextMetric struct {
	name   string
	result metric.Result
	calls  int
}

func (m *staticContextMetric) Name() string { return m.name }

func (m *staticContextMetric) Compute(_ context.Context, _ []bench.ScoredSample[string]) (metric.Result, error) {
	m.calls++
	return m.result, nil
}

func TestAsSyncEqualsContextMetricResult(t *testing.T) {
	t.Parallel()

	want := metric.Result{Name: "semantic", Value: 0.75, Values: map[string]float64{"avg_similarity": 0.75}}
	cm := &staticContextMetric{name: "semantic", result: want}

	got, err := cm.Compute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	// The precompute path must yield an identical Result to the context-metric path.
	sync := metric.AsSync[string](got)
	if sync.Name() != want.Name {
		t.Errorf("AsSync Name = %q, want %q", sync.Name(), want.Name)
	}
	syncRes := sync.Compute([]bench.ScoredSample[string]{{}})
	if syncRes.Value != want.Value || syncRes.Values["avg_similarity"] != 0.75 {
		t.Errorf("AsSync Result = %+v, want %+v", syncRes, want)
	}
}

func TestAsRunContextMetricPropagatesResult(t *testing.T) {
	t.Parallel()

	cm := &staticContextMetric{name: "semantic", result: metric.Result{Name: "semantic", Value: 0.5, Descriptive: true}}
	rm := metric.AsRunContextMetric[string](cm)

	out, err := rm.Compute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if out.Name != "semantic" || out.Value != 0.5 || !out.Descriptive {
		t.Errorf("RunContextMetric Result = %+v, want name=semantic value=0.5 descriptive=true", out)
	}
}
