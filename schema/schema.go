package schema

import (
	"encoding/json"
	"reflect"

	"github.com/invopop/jsonschema"
)

// JSON is a standard JSON Schema document represented as a map.
//
// The map[string]any representation is a deliberate, documented opaque-value
// exception to the no-any rule: a JSON Schema document is arbitrary JSON and
// must stay format-agnostic so it serializes cleanly to any wire format
// (OpenAI, Anthropic, MCP, etc.).
type JSON = map[string]any

// Generate creates a JSON Schema from a Go type using struct tags.
// The type parameter T should be a struct with json and optional
// jsonschema tags.
//
//	schema.Generate[SearchInput]()
//	schema.Generate[SearchInput](schema.WithTitle("Search"), schema.WithDescription("..."))
func Generate[T any](opts ...Option) JSON {
	cfg := applyOptions(opts)
	r := newReflector(cfg)
	s := r.Reflect(new(T))
	applyOverrides(s, cfg)
	return toJSON(s)
}

// From creates a JSON Schema from a reflect.Type. Use this when the
// type is not known at compile time.
func From(t reflect.Type, opts ...Option) JSON {
	cfg := applyOptions(opts)
	r := newReflector(cfg)
	s := r.ReflectFromType(t)
	applyOverrides(s, cfg)
	return toJSON(s)
}

// newReflector creates a configured jsonschema.Reflector for tool-oriented
// schema generation.
func newReflector(cfg *config) *jsonschema.Reflector {
	return &jsonschema.Reflector{
		Anonymous:                  true,
		DoNotReference:             cfg.inline,
		AllowAdditionalProperties:  cfg.allowAdditional,
		RequiredFromJSONSchemaTags: true,
	}
}

// applyOverrides sets title and description on the root schema if provided.
func applyOverrides(s *jsonschema.Schema, cfg *config) {
	if cfg.title != "" {
		s.Title = cfg.title
	}
	if cfg.description != "" {
		s.Description = cfg.description
	}
}

// toJSON converts a *jsonschema.Schema to a plain map[string]any.
func toJSON(s *jsonschema.Schema) JSON {
	b, err := json.Marshal(s)
	if err != nil {
		return JSON{"type": "object"}
	}
	var m JSON
	if err := json.Unmarshal(b, &m); err != nil {
		return JSON{"type": "object"}
	}
	return m
}
