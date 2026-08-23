package discovery

import "testing"

func inst(id string, weight int) ServiceInstance {
	return ServiceInstance{ID: id, Name: "svc", Address: "127.0.0.1", Port: 8080, Weight: weight, Health: HealthHealthy}
}

func TestRoundRobinBalancer_Cycles(t *testing.T) {
	t.Parallel()
	b := NewRoundRobinBalancer()
	instances := []ServiceInstance{inst("a", 1), inst("b", 1), inst("c", 1)}
	want := []string{"a", "b", "c", "a"}
	for i, w := range want {
		got, ok := b.Pick(instances)
		if !ok || got.ID != w {
			t.Fatalf("pick %d = %q (ok=%v), want %q", i, got.ID, ok, w)
		}
	}
}

func TestBalancers_EmptyReturnsFalse(t *testing.T) {
	t.Parallel()
	for name, b := range map[string]Balancer{
		"roundrobin":  NewRoundRobinBalancer(),
		"random":      NewRandomBalancer(),
		"weighted":    NewWeightedBalancer(),
		"leastconn":   NewLeastConnectionsBalancer(),
		"factoryrand": NewBalancer("unknown"),
	} {
		if _, ok := b.Pick(nil); ok {
			t.Errorf("%s: expected ok=false on empty", name)
		}
	}
}

func TestRandomAndWeighted_ReturnMember(t *testing.T) {
	t.Parallel()
	instances := []ServiceInstance{inst("a", 1), inst("b", 5)}
	for _, b := range []Balancer{NewRandomBalancer(), NewWeightedBalancer()} {
		got, ok := b.Pick(instances)
		if !ok || (got.ID != "a" && got.ID != "b") {
			t.Fatalf("pick = %q (ok=%v), want a or b", got.ID, ok)
		}
	}
}

func TestWeightedBalancer_ZeroWeightTreatedAsOne(t *testing.T) {
	t.Parallel()
	b := NewWeightedBalancer()
	// Single zero-weight instance: total normalizes to 1, must not panic or divide by zero.
	got, ok := b.Pick([]ServiceInstance{inst("only", 0)})
	if !ok || got.ID != "only" {
		t.Fatalf("pick = %q (ok=%v), want only", got.ID, ok)
	}
}

func TestLeastConnectionsBalancer_PrefersIdle(t *testing.T) {
	t.Parallel()
	b := NewLeastConnectionsBalancer()
	instances := []ServiceInstance{inst("a", 1), inst("b", 1)}

	b.Acquire("a")
	b.Acquire("a")
	b.Acquire("b")

	got, ok := b.Pick(instances)
	if !ok || got.ID != "b" {
		t.Fatalf("pick = %q (ok=%v), want b (fewest in-flight)", got.ID, ok)
	}

	// Releasing a back to idle makes it the least-loaded again.
	b.Release("a")
	b.Release("a")
	got, _ = b.Pick(instances)
	if got.ID != "a" {
		t.Fatalf("after release pick = %q, want a", got.ID)
	}
}

func TestLeastConnectionsBalancer_ReleaseNeverNegative(t *testing.T) {
	t.Parallel()
	b := NewLeastConnectionsBalancer()
	b.Release("ghost") // no prior acquire; must be a no-op, not negative
	b.Acquire("ghost")
	b.Release("ghost")
	b.Release("ghost") // extra release stays at zero
	got, ok := b.Pick([]ServiceInstance{inst("ghost", 1)})
	if !ok || got.ID != "ghost" {
		t.Fatalf("pick = %q (ok=%v)", got.ID, ok)
	}
}

func TestNewBalancer_SelectsType(t *testing.T) {
	t.Parallel()
	cases := map[LoadBalancingStrategy]any{
		StrategyRoundRobin: (*RoundRobinBalancer)(nil),
		StrategyWeighted:   (*WeightedBalancer)(nil),
		StrategyLeastConn:  (*LeastConnectionsBalancer)(nil),
		StrategyRandom:     (*RandomBalancer)(nil),
	}
	for strategy, want := range cases {
		got := NewBalancer(strategy)
		switch want.(type) {
		case *RoundRobinBalancer:
			if _, ok := got.(*RoundRobinBalancer); !ok {
				t.Errorf("%s: got %T", strategy, got)
			}
		case *WeightedBalancer:
			if _, ok := got.(*WeightedBalancer); !ok {
				t.Errorf("%s: got %T", strategy, got)
			}
		case *LeastConnectionsBalancer:
			if _, ok := got.(*LeastConnectionsBalancer); !ok {
				t.Errorf("%s: got %T", strategy, got)
			}
		case *RandomBalancer:
			if _, ok := got.(*RandomBalancer); !ok {
				t.Errorf("%s: got %T", strategy, got)
			}
		}
	}
}
