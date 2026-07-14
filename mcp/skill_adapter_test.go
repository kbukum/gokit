package mcp_test

import (
	"context"
	"testing"

	kitMcp "github.com/kbukum/gokit/mcp"
	"github.com/kbukum/gokit/skill"
	"github.com/kbukum/gokit/tool"
)

func validManifest(tools ...string) skill.Manifest {
	return skill.Manifest{
		SchemaVersion: "1.0.0",
		Name:          "inspector",
		Version:       "1.0.0",
		Description:   "inspects things",
		References:    skill.References{Tools: tools},
	}
}

func TestSkillAdapterPinsAllowList(t *testing.T) {
	ctx := context.Background()
	reg := newTestRegistry(t) // add, greet, fail
	adapter := kitMcp.SkillToServerAdapter{Manifest: validManifest("greet"), Registry: reg}
	server, err := adapter.NewServer("skill-server", "1.0.0")
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	callables := callablesByName(serveClient(t, ctx, server, nil))
	if len(callables) != 1 {
		t.Fatalf("expected only the referenced tool, got %d: %v", len(callables), callables)
	}
	if _, ok := callables["greet"]; !ok {
		t.Errorf("expected 'greet' to be exposed, got %v", callables)
	}
}

func TestSkillAdapterRejectsNilRegistry(t *testing.T) {
	t.Parallel()
	adapter := kitMcp.SkillToServerAdapter{Manifest: validManifest("greet")}
	if _, err := adapter.NewServer("s", "1.0.0"); err == nil {
		t.Fatal("expected error for nil registry")
	}
}

func TestSkillAdapterRejectsInvalidManifest(t *testing.T) {
	t.Parallel()
	adapter := kitMcp.SkillToServerAdapter{
		Manifest: skill.Manifest{Name: "missing-fields"},
		Registry: tool.NewRegistry(),
	}
	if _, err := adapter.NewServer("s", "1.0.0"); err == nil {
		t.Fatal("expected validation error for invalid manifest")
	}
}
