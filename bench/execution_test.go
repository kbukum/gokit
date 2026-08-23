package bench

import "testing"

func TestNewExecutionPlanNormalizesConcurrency(t *testing.T) {
	t.Parallel()

	if got := NewExecutionPlan(0).Concurrency; got != 1 {
		t.Errorf("Concurrency = %d, want 1", got)
	}
	if got := NewExecutionPlan(-3).Concurrency; got != 1 {
		t.Errorf("Concurrency = %d, want 1", got)
	}
	if got := NewExecutionPlan(3).Concurrency; got != 3 {
		t.Errorf("Concurrency = %d, want 3", got)
	}
}
