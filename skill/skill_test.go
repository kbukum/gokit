package skill_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/skill"
	"github.com/kbukum/gokit/tool"
	"github.com/kbukum/gokit/util"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := util.WriteFile(path, []byte(body)); err != nil {
		t.Fatal(err)
	}
}

const manifest = `schema_version: "1"
name: demo
version: 0.1.0
description: Demo skill
license: MIT
authors: [Ada]
references:
  tools: [read, mutate]
  prompts:
    - name: demo
      version: 0.1.0
  resources: [file://docs]
  mcp_servers: [local]
requires:
  scopes: [skill:activate]
  capabilities: [filesystem]
human_approval:
  - step: publish
    when: before external publish
    rationale: publishing mutates external state
budgets:
  max_tokens: 100000
  max_calls: 50
  max_cost: {amount: 1.25, currency: USD}
  wall_clock: PT60S
model_hints:
  preferred: [test]
  reject: [slow]
progressive_disclosure:
  summary: Short summary
  detail: Detailed instructions
scripts:
  - path: scripts/run.sh
    description: inert helper
signature:
  algorithm: ed25519
  value: abc
  key_id: test-key
safety: read_only
`

func TestLoadManifestRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest+"humanApproval: []\n")
	if _, err := skill.LoadManifest(filepath.Join(dir, skill.ManifestFileName)); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestLoadManifestCanonicalSchema(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	m, err := skill.LoadManifest(filepath.Join(dir, skill.ManifestFileName))
	if err != nil {
		t.Fatal(err)
	}
	if m.SchemaVersion != "1" || m.References.MCPServers[0] != "local" || m.HumanApproval[0].Step != "publish" {
		t.Fatalf("manifest=%+v", m)
	}
}

func TestLoaderEnumeratesInertAssets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	writeFile(t, filepath.Join(dir, "SKILL.md"), "---\ntitle: Demo\n---\n# Demo")
	writeFile(t, filepath.Join(dir, "references", "a.md"), "ref")
	writeFile(t, filepath.Join(dir, "scripts", "run.sh"), "echo nope")
	pack, err := skill.NewLoader().Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pack.SkillBody != "# Demo" {
		t.Fatalf("body=%q", pack.SkillBody)
	}
	if len(pack.References) != 1 || len(pack.Scripts) != 1 {
		t.Fatalf("assets refs=%d scripts=%d", len(pack.References), len(pack.Scripts))
	}
	b, err := pack.LoadReferenceDoc("a.md")
	if err != nil || string(b) != "ref" {
		t.Fatalf("reference %q err %v", string(b), err)
	}
}

func TestEffectiveSafetyAndEnvelope(t *testing.T) {
	m := skill.Manifest{Safety: skill.SafetyDestructive, References: skill.References{Tools: []string{"read", "delete"}}}
	got := skill.EffectiveSafety(m, func(name string) tool.Safety {
		if name == "delete" {
			return tool.SafetyMutating
		}
		return tool.SafetyReadOnly
	})
	if got != tool.SafetyDestructive {
		t.Fatalf("safety=%s", got)
	}
	envs := map[string]tool.Envelope{"read": {Scopes: []string{"db:read"}}, "delete": {Scopes: []string{"db:delete"}}}
	eff := skill.EffectiveEnvelope(m, []string{"db:read"}, []string{"db:read", "db:delete"}, envs)
	if !eff[0].Allowed || eff[1].Allowed {
		t.Fatalf("effective=%+v", eff)
	}
}

type provider struct{ manifest skill.Manifest }

func (p provider) Manifest() skill.Manifest                { return p.manifest }
func (p provider) OpenAsset(string) (io.ReadCloser, error) { return nil, os.ErrNotExist }

func validManifest() skill.Manifest {
	return skill.Manifest{SchemaVersion: "1", Name: "demo", Version: "0.1.0", Description: "d"}
}

func TestRegistryRegister(t *testing.T) {
	reg := skill.NewRegistry()
	if err := reg.Register(provider{manifest: validManifest()}); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("demo"); !ok {
		t.Fatal("missing provider")
	}
	if len(reg.List()) != 1 {
		t.Fatal("list length")
	}
}

func TestRegistryListSorted(t *testing.T) {
	reg := skill.NewRegistry()
	for _, manifest := range []skill.Manifest{
		{SchemaVersion: "1", Name: "zeta", Version: "0.1.0", Description: "z"},
		{SchemaVersion: "1", Name: "alpha", Version: "0.1.0", Description: "a"},
	} {
		if err := reg.Register(provider{manifest: manifest}); err != nil {
			t.Fatalf("Register(%s): %v", manifest.Name, err)
		}
	}
	got := reg.List()
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "zeta" {
		t.Fatalf("list=%+v", got)
	}
}

func TestManifestValidationFailures(t *testing.T) {
	cases := []skill.Manifest{
		{},
		{SchemaVersion: "1", Name: "demo", Version: "bad", Description: "d"},
		{SchemaVersion: "1", Name: "demo", Version: "0.1.0", Description: ""},
		{SchemaVersion: "1", Name: "demo", Version: "0.1.0", Description: "d", Safety: "bad"},
		{SchemaVersion: "1", Name: "demo", Version: "0.1.0", Description: "d", References: skill.References{Tools: []string{"x", "x"}}},
		{SchemaVersion: "1", Name: "demo", Version: "0.1.0", Description: "d", HumanApproval: []skill.HumanApproval{{}}},
	}
	for i := range cases {
		if err := skill.Validate(&cases[i]); err == nil {
			t.Fatalf("case %d expected error", i)
		}
	}
}

func TestLoaderEdgeCasesAndVerifier(t *testing.T) {
	if err := skill.Validate(nil); err == nil {
		t.Fatal("nil manifest should fail")
	}
	if err := (skill.WarnOnlyVerifier{}).Verify(nil, skill.Signature{}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	writeFile(t, filepath.Join(dir, "SKILL.md"), "body")
	writeFile(t, filepath.Join(dir, "scripts", "run.sh"), "echo nope")
	pack, err := skill.NewLoader().Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.ListScripts()) != 1 {
		t.Fatal("expected script")
	}
	if _, err := pack.LoadReferenceDoc("../bad"); err == nil {
		t.Fatal("expected path traversal error")
	}
	writeFile(t, filepath.Join(dir, "references"), "not dir")
	if _, err := skill.NewLoader().Load(dir); err == nil {
		t.Fatal("expected references dir error")
	}
}

func TestRegistryValidationFailures(t *testing.T) {
	reg := skill.NewRegistry()
	if err := reg.Register(nil); err == nil {
		t.Fatal("nil provider")
	}
	m := validManifest()
	if err := reg.Register(provider{manifest: m}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(provider{manifest: m}); err == nil {
		t.Fatal("duplicate provider")
	}
}
