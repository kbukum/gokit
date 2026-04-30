package dag

import (
	"context"
	"errors"
	"testing"
)

func TestEngineConfig_FailurePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		engine        *Engine
		wantTopErr    bool
		wantDependent string
	}{
		{
			name:       "fail fast",
			engine:     NewEngine(EngineConfig{FailurePolicy: FailFast}),
			wantTopErr: true,
		},
		{
			name:          "continue",
			engine:        NewEngine(EngineConfig{FailurePolicy: Continue}),
			wantDependent: StatusCompleted,
		},
		{
			name:          "skip dependents",
			engine:        NewEngine(EngineConfig{FailurePolicy: SkipDependents}),
			wantDependent: StatusDepFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := &Graph{
				Nodes: map[string]Node{
					"a": newFuncNode("a", func(_ context.Context, _ *State) (any, error) {
						return nil, errors.New("boom")
					}),
					"b": newFuncNode("b", func(_ context.Context, s *State) (any, error) {
						s.Set("b", true)
						return "ok", nil
					}),
				},
				Edges: []Edge{{From: "a", To: "b"}},
			}

			result, err := tt.engine.ExecuteBatch(context.Background(), g, NewState())
			if tt.wantTopErr {
				if err == nil {
					t.Fatal("expected top-level error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := result.NodeResults["b"].Status; got != tt.wantDependent {
				t.Fatalf("dependent status = %s, want %s", got, tt.wantDependent)
			}
		})
	}
}

func TestEngine_LegacyDefaultStillSkipsDependents(t *testing.T) {
	t.Parallel()

	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", func(_ context.Context, _ *State) (any, error) {
				return nil, errors.New("boom")
			}),
			"b": newFuncNode("b", func(_ context.Context, _ *State) (any, error) {
				return "ok", nil
			}),
		},
		Edges: []Edge{{From: "a", To: "b"}},
	}

	result, err := (&Engine{}).ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := result.NodeResults["b"].Status; got != StatusDepFailed {
		t.Fatalf("status = %s, want %s", got, StatusDepFailed)
	}
}

func TestEngine_NodePolicyOverridesEnginePolicy(t *testing.T) {
	t.Parallel()

	g := &Graph{
		Nodes: map[string]Node{
			"a": newFuncNode("a", func(_ context.Context, _ *State) (any, error) {
				return nil, errors.New("boom")
			}),
			"b": newFuncNode("b", func(_ context.Context, _ *State) (any, error) {
				return "ok", nil
			}),
		},
		Edges: []Edge{{From: "a", To: "b"}},
		NodeDefs: map[string]NodeDef{
			"a": {Component: "a", OnError: OnErrorContinue},
		},
	}

	result, err := NewEngine(EngineConfig{FailurePolicy: SkipDependents}).ExecuteBatch(context.Background(), g, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := result.NodeResults["b"].Status; got != StatusCompleted {
		t.Fatalf("status = %s, want %s", got, StatusCompleted)
	}
}
