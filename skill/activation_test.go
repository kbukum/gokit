package skill_test

import (
	"testing"

	"github.com/kbukum/gokit/skill"
	"github.com/kbukum/gokit/tool"
)

func TestActivateAllowedWhenScopesGranted(t *testing.T) {
	m := skill.Manifest{
		Name:       "demo",
		Safety:     skill.SafetyReadOnly,
		References: skill.References{Tools: []string{"read", "delete"}},
	}
	envs := map[string]tool.Envelope{
		"read":   {Scopes: []string{"db:read"}, Safety: tool.SafetyReadOnly},
		"delete": {Scopes: []string{"db:delete"}, Safety: tool.SafetyDestructive},
	}
	grants := []string{"db:read", "db:delete"}
	ceiling := []string{"db:read", "db:delete"}

	got := skill.Activate(m, grants, ceiling, envs)
	if got.SkillName != "demo" {
		t.Fatalf("skill name=%q", got.SkillName)
	}
	if !got.Allowed {
		t.Fatalf("expected allowed, reason=%q", got.Reason)
	}
	if got.EffectiveSafety != tool.SafetyDestructive {
		t.Fatalf("effective safety=%s", got.EffectiveSafety)
	}
	if len(got.Tools) != 2 || !got.Tools[0].Allowed || !got.Tools[1].Allowed {
		t.Fatalf("tools=%+v", got.Tools)
	}
}

func TestActivateDeniedWhenScopeMissing(t *testing.T) {
	m := skill.Manifest{
		Name:       "demo",
		References: skill.References{Tools: []string{"read", "delete"}},
	}
	envs := map[string]tool.Envelope{
		"read":   {Scopes: []string{"db:read"}},
		"delete": {Scopes: []string{"db:delete"}},
	}
	// Principal lacks db:delete, so the delete tool cannot activate.
	got := skill.Activate(m, []string{"db:read"}, []string{"db:read", "db:delete"}, envs)
	if got.Allowed {
		t.Fatalf("expected denied when a referenced tool loses a scope")
	}
	if got.Reason == "" {
		t.Fatal("expected a denial reason")
	}
}

func TestActivateDeniedWhenEnvelopeMissing(t *testing.T) {
	m := skill.Manifest{Name: "demo", References: skill.References{Tools: []string{"ghost"}}}
	got := skill.Activate(m, nil, nil, map[string]tool.Envelope{})
	if got.Allowed {
		t.Fatal("expected denied for missing tool envelope")
	}
}

func TestEffectiveSafetyMutatingManifest(t *testing.T) {
	// A mutating manifest floor holds even when every referenced tool is
	// read-only, exercising the SafetyMutating mapping.
	m := skill.Manifest{Safety: skill.SafetyMutating, References: skill.References{Tools: []string{"read"}}}
	got := skill.EffectiveSafety(m, func(string) tool.Safety { return tool.SafetyReadOnly })
	if got != tool.SafetyMutating {
		t.Fatalf("effective safety=%s, want mutating", got)
	}
}
