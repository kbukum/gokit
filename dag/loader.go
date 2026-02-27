package dag

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// PipelineLoader loads pipeline definitions by name.
type PipelineLoader interface {
	Load(name string) (*Pipeline, error)
}

// FilePipelineLoader loads pipelines from YAML files on disk.
type FilePipelineLoader struct {
	dirs []string
}

// NewFilePipelineLoader creates a loader that searches the given directories for pipeline YAML files.
func NewFilePipelineLoader(dirs ...string) PipelineLoader {
	return &FilePipelineLoader{dirs: dirs}
}

// Load searches for a pipeline YAML file by name across configured directories.
// It searches for {name}.yaml and {name}.yml in each directory (recursively).
func (l *FilePipelineLoader) Load(name string) (*Pipeline, error) {
	for _, dir := range l.dirs {
		for _, ext := range []string{".yaml", ".yml"} {
			// Try direct path first
			path := filepath.Join(dir, name+ext)
			if p, err := loadPipelineFile(path); err == nil {
				return p, nil
			}

			// Search subdirectories
			matches, _ := filepath.Glob(filepath.Join(dir, "**", name+ext))
			for _, match := range matches {
				if p, err := loadPipelineFile(match); err == nil {
					return p, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("dag: pipeline %q not found in %v", name, l.dirs)
}

func loadPipelineFile(path string) (*Pipeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Pipeline
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("dag: parsing %s: %w", path, err)
	}
	return &p, nil
}

// LoadPipeline loads a pipeline from explicit file paths.
// It tries each path until one succeeds.
func LoadPipeline(name string, paths ...string) (*Pipeline, error) {
	for _, path := range paths {
		p, err := loadPipelineFile(path)
		if err == nil {
			return p, nil
		}
	}
	return nil, fmt.Errorf("dag: pipeline %q not found in provided paths", name)
}

// ResolvePipeline converts a Pipeline definition into an executable Graph.
// It resolves includes recursively and looks up node implementations from the registry.
func ResolvePipeline(p *Pipeline, registry *Registry, loader PipelineLoader) (*Graph, error) {
	stack := make(map[string]bool)    // current recursion path (cycle detection)
	resolved := make(map[string]bool) // already fully resolved (dedup)
	return resolvePipeline(p, registry, loader, stack, resolved)
}

func resolvePipeline(p *Pipeline, registry *Registry, loader PipelineLoader, stack, resolved map[string]bool) (*Graph, error) {
	if stack[p.Name] {
		return nil, fmt.Errorf("dag: circular include detected for pipeline %q", p.Name)
	}
	stack[p.Name] = true
	defer delete(stack, p.Name)

	g := &Graph{
		Nodes: make(map[string]Node),
	}

	// Resolve includes first
	for _, includeName := range p.Includes {
		if resolved[includeName] {
			continue // already resolved in a different branch (diamond)
		}

		sub, err := loader.Load(includeName)
		if err != nil {
			return nil, fmt.Errorf("dag: loading include %q: %w", includeName, err)
		}

		subGraph, err := resolvePipeline(sub, registry, loader, stack, resolved)
		if err != nil {
			return nil, err
		}

		// Merge sub-graph into main graph
		for name, node := range subGraph.Nodes {
			if _, exists := g.Nodes[name]; exists {
				continue // dedup: first wins (diamond includes)
			}
			g.Nodes[name] = node
		}
		g.Edges = append(g.Edges, subGraph.Edges...)
	}

	// Resolve this pipeline's nodes
	for _, def := range p.Nodes {
		if _, exists := g.Nodes[def.Component]; exists {
			continue // already added via include
		}

		node, ok := registry.Get(def.Component)
		if !ok {
			return nil, fmt.Errorf("dag: component %q not found in registry", def.Component)
		}
		g.Nodes[def.Component] = node

		for _, dep := range def.DependsOn {
			g.Edges = append(g.Edges, Edge{From: dep, To: def.Component})
		}
	}

	resolved[p.Name] = true
	return g, nil
}
