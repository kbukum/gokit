package dag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPipeline_FromFile(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: test-pipeline
mode: batch
nodes:
  - component: extract
  - component: transform
    depends_on: [extract]
  - component: load
    depends_on: [transform]
`
	path := filepath.Join(dir, "test-pipeline.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadPipeline("test", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "test-pipeline" {
		t.Fatalf("expected 'test-pipeline', got %q", p.Name)
	}
	if len(p.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(p.Nodes))
	}
}

func TestFilePipelineLoader_Load(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
name: my-pipe
mode: batch
nodes:
  - component: step1
`
	if err := os.WriteFile(filepath.Join(dir, "my-pipe.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := NewFilePipelineLoader(dir)
	p, err := loader.Load("my-pipe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "my-pipe" {
		t.Fatalf("expected 'my-pipe', got %q", p.Name)
	}
}

func TestFilePipelineLoader_NotFound(t *testing.T) {
	loader := NewFilePipelineLoader(t.TempDir())
	_, err := loader.Load("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolvePipeline_Basic(t *testing.T) {
	reg := NewRegistry()
	reg.Register("extract", newFuncNode("extract", func(_ context.Context, s *State) (any, error) {
		s.Set("data", "raw")
		return "raw", nil
	}))
	reg.Register("transform", newFuncNode("transform", func(_ context.Context, s *State) (any, error) {
		return "transformed", nil
	}))

	p := &Pipeline{
		Name: "test",
		Mode: "batch",
		Nodes: []NodeDef{
			{Component: "extract"},
			{Component: "transform", DependsOn: []string{"extract"}},
		},
	}

	g, err := ResolvePipeline(p, reg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.Edges))
	}
}

func TestResolvePipeline_MissingComponent(t *testing.T) {
	reg := NewRegistry()
	p := &Pipeline{
		Name:  "test",
		Nodes: []NodeDef{{Component: "missing"}},
	}

	_, err := ResolvePipeline(p, reg, nil)
	if err == nil {
		t.Fatal("expected error for missing component")
	}
}

func TestResolvePipeline_WithIncludes(t *testing.T) {
	reg := NewRegistry()
	reg.Register("a", newFuncNode("a", nil))
	reg.Register("b", newFuncNode("b", nil))
	reg.Register("c", newFuncNode("c", nil))

	sub := &Pipeline{
		Name: "sub",
		Nodes: []NodeDef{
			{Component: "a"},
			{Component: "b", DependsOn: []string{"a"}},
		},
	}

	main := &Pipeline{
		Name:     "main",
		Includes: []string{"sub"},
		Nodes: []NodeDef{
			{Component: "c", DependsOn: []string{"b"}},
		},
	}

	loader := &memoryLoader{pipelines: map[string]*Pipeline{"sub": sub}}
	g, err := ResolvePipeline(main, reg, loader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(g.Nodes))
	}
}

func TestResolvePipeline_CircularInclude(t *testing.T) {
	reg := NewRegistry()
	reg.Register("a", newFuncNode("a", nil))

	loader := &memoryLoader{pipelines: map[string]*Pipeline{
		"alpha": {
			Name:     "alpha",
			Includes: []string{"beta"},
			Nodes:    []NodeDef{{Component: "a"}},
		},
		"beta": {
			Name:     "beta",
			Includes: []string{"alpha"},
			Nodes:    []NodeDef{{Component: "a"}},
		},
	}}

	_, err := ResolvePipeline(loader.pipelines["alpha"], reg, loader)
	if err == nil {
		t.Fatal("expected circular include error")
	}
}

func TestResolvePipeline_DiamondIncludes(t *testing.T) {
	reg := NewRegistry()
	reg.Register("shared", newFuncNode("shared", nil))
	reg.Register("left", newFuncNode("left", nil))
	reg.Register("right", newFuncNode("right", nil))

	shared := &Pipeline{
		Name:  "shared-pipe",
		Nodes: []NodeDef{{Component: "shared"}},
	}
	leftPipe := &Pipeline{
		Name:     "left-pipe",
		Includes: []string{"shared-pipe"},
		Nodes:    []NodeDef{{Component: "left", DependsOn: []string{"shared"}}},
	}
	rightPipe := &Pipeline{
		Name:     "right-pipe",
		Includes: []string{"shared-pipe"},
		Nodes:    []NodeDef{{Component: "right", DependsOn: []string{"shared"}}},
	}

	loader := &memoryLoader{pipelines: map[string]*Pipeline{
		"shared-pipe": shared,
		"left-pipe":   leftPipe,
		"right-pipe":  rightPipe,
	}}

	main := &Pipeline{
		Name:     "main",
		Includes: []string{"left-pipe", "right-pipe"},
	}

	g, err := ResolvePipeline(main, reg, loader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// shared, left, right — shared is deduped
	if len(g.Nodes) != 3 {
		t.Fatalf("expected 3 nodes (deduped), got %d", len(g.Nodes))
	}
}

// memoryLoader is a test helper for in-memory pipeline loading.
type memoryLoader struct {
	pipelines map[string]*Pipeline
}

func (m *memoryLoader) Load(name string) (*Pipeline, error) {
	p, ok := m.pipelines[name]
	if !ok {
		return nil, fmt.Errorf("pipeline %q not found", name)
	}
	return p, nil
}
