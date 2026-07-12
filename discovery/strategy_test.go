package discovery

import "testing"

// ── Query and strategy types ─────────────────────────────────────────

func TestLoadBalancingStrategy_Constants(t *testing.T) {
	if Random != StrategyRandom {
		t.Error("Random alias mismatch")
	}
	if RoundRobin != StrategyRoundRobin {
		t.Error("RoundRobin alias mismatch")
	}
	if Weighted != StrategyWeighted {
		t.Error("Weighted alias mismatch")
	}
	if LeastConn != StrategyLeastConn {
		t.Error("LeastConn alias mismatch")
	}
}

func TestCriticality_Constants(t *testing.T) {
	if CriticalityRequired != "required" {
		t.Error("CriticalityRequired mismatch")
	}
	if CriticalityOptional != "optional" {
		t.Error("CriticalityOptional mismatch")
	}
}
