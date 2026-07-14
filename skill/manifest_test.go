package skill_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/skill"
)

func TestValidateFieldFailures(t *testing.T) {
	base := func() skill.Manifest {
		return skill.Manifest{SchemaVersion: "1", Name: "demo", Version: "0.1.0", Description: "d"}
	}
	cases := []struct {
		name   string
		mutate func(*skill.Manifest)
	}{
		{"missing name", func(m *skill.Manifest) { m.Name = "" }},
		{"non-semver version", func(m *skill.Manifest) { m.Version = "one" }},
		{"missing description", func(m *skill.Manifest) { m.Description = "" }},
		{"duplicate tools", func(m *skill.Manifest) { m.References.Tools = []string{"a", "a"} }},
		{"empty tool value", func(m *skill.Manifest) { m.References.Tools = []string{"  "} }},
		{"duplicate resources", func(m *skill.Manifest) { m.References.Resources = []string{"r", "r"} }},
		{"duplicate mcp servers", func(m *skill.Manifest) { m.References.MCPServers = []string{"s", "s"} }},
		{"duplicate scopes", func(m *skill.Manifest) { m.Requires.Scopes = []string{"x", "x"} }},
		{"duplicate capabilities", func(m *skill.Manifest) { m.Requires.Capabilities = []string{"c", "c"} }},
		{"prompt missing version", func(m *skill.Manifest) {
			m.References.Prompts = []skill.PromptReference{{Name: "p"}}
		}},
		{"duplicate prompt", func(m *skill.Manifest) {
			m.References.Prompts = []skill.PromptReference{{Name: "p", Version: "1"}, {Name: "p", Version: "1"}}
		}},
		{"human approval incomplete", func(m *skill.Manifest) {
			m.HumanApproval = []skill.HumanApproval{{Step: "s"}}
		}},
		{"script missing description", func(m *skill.Manifest) {
			m.Scripts = []skill.Script{{Path: "scripts/a.sh"}}
		}},
		{"absolute script path", func(m *skill.Manifest) {
			m.Scripts = []skill.Script{{Path: "/abs.sh", Description: "d"}}
		}},
		{"invalid safety", func(m *skill.Manifest) { m.Safety = "spicy" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := base()
			tc.mutate(&m)
			if err := skill.Validate(&m); !errors.Is(err, skill.ErrManifestInvalid) {
				t.Fatalf("want ErrManifestInvalid, got %v", err)
			}
		})
	}
}

func TestLoadManifestMissingFile(t *testing.T) {
	if _, err := skill.LoadManifest(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("expected error for a missing manifest file")
	}
}

func TestLoadManifestUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, skill.ManifestFileName)
	writeFile(t, path, manifest)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	if _, err := skill.LoadManifest(path); err == nil {
		t.Skip("file remained readable; cannot exercise the open-error path")
	}
}
