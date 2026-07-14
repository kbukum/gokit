package skill_test

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kbukum/gokit/fs"
	"github.com/kbukum/gokit/skill"
	"github.com/kbukum/gokit/util"
)

func writePack(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	writeFile(t, filepath.Join(dir, "SKILL.md"), "---\ntitle: Demo\n---\n# Demo")
	writeFile(t, filepath.Join(dir, "references", "a.md"), "ref")
	writeFile(t, filepath.Join(dir, "scripts", "run.sh"), "echo nope")
}

func TestValidateWrapsSentinel(t *testing.T) {
	err := skill.Validate(&skill.Manifest{})
	if !errors.Is(err, skill.ErrManifestInvalid) {
		t.Fatalf("want ErrManifestInvalid, got %v", err)
	}
}

func TestParseManifestRejectsMalformed(t *testing.T) {
	if _, err := skill.ParseManifest([]byte("name: [bad")); !errors.Is(err, skill.ErrParseManifest) {
		t.Fatalf("want ErrParseManifest, got %v", err)
	}
}

func TestValidateRejectsUnsafeScriptPath(t *testing.T) {
	m := validManifest()
	m.Scripts = []skill.Script{{Path: "../evil.sh", Description: "escape"}}
	if err := skill.Validate(&m); !errors.Is(err, skill.ErrManifestInvalid) {
		t.Fatalf("want ErrManifestInvalid for escaping script, got %v", err)
	}
}

func TestLoadManifestRejectsOversizedManifest(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("a", skill.MaxManifestBytes+1)
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), big)
	_, err := skill.LoadManifest(filepath.Join(dir, skill.ManifestFileName))
	if !errors.Is(err, skill.ErrFileTooLarge) {
		t.Fatalf("want ErrFileTooLarge, got %v", err)
	}
}

func TestLoaderRejectsOversizedBody(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	writeFile(t, filepath.Join(dir, "SKILL.md"), strings.Repeat("x", skill.MaxBodyBytes+1))
	_, err := skill.NewLoader().Load(dir)
	if !errors.Is(err, skill.ErrFileTooLarge) {
		t.Fatalf("want ErrFileTooLarge, got %v", err)
	}
}

func TestLoaderRejectsInvalidUTF8Body(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	if err := util.WriteFile(filepath.Join(dir, "SKILL.md"), []byte{0xff, 0xfe}); err != nil {
		t.Fatal(err)
	}
	_, err := skill.NewLoader().Load(dir)
	if !errors.Is(err, skill.ErrInvalidUTF8) {
		t.Fatalf("want ErrInvalidUTF8, got %v", err)
	}
}

func TestLoaderDenyVerifierFailsClosed(t *testing.T) {
	dir := t.TempDir()
	writePack(t, dir)
	_, err := skill.NewLoader(skill.WithVerifier(skill.DenyVerifier{})).Load(dir)
	if !errors.Is(err, skill.ErrVerificationDenied) {
		t.Fatalf("want ErrVerificationDenied, got %v", err)
	}
	if _, _, err := skill.NewLoader(skill.WithVerifier(skill.DenyVerifier{})).LoadMetadata(dir); !errors.Is(err, skill.ErrVerificationDenied) {
		t.Fatalf("LoadMetadata want ErrVerificationDenied, got %v", err)
	}
}

func TestLoaderCollectsWarningsForUnsignedManifest(t *testing.T) {
	dir := t.TempDir()
	// Manifest without a signature triggers the warn-only path.
	head, _, _ := strings.Cut(manifest, "signature:")
	unsigned := head + "safety: read-only\n"
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), unsigned)
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# body")
	writeFile(t, filepath.Join(dir, "scripts", "run.sh"), "echo nope")
	pack, err := skill.NewLoader().Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.VerificationWarnings) != 1 || pack.VerificationWarnings[0] != "unsigned skill manifest" {
		t.Fatalf("warnings=%v", pack.VerificationWarnings)
	}
}

func TestLoaderSignedManifestHasNoWarnings(t *testing.T) {
	dir := t.TempDir()
	writePack(t, dir)
	pack, err := skill.NewLoader().Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.VerificationWarnings) != 0 {
		t.Fatalf("unexpected warnings=%v", pack.VerificationWarnings)
	}
}

func TestLoaderRejectsSymlinkedReferencesDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# body")
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.txt"), "secret")
	if err := os.Symlink(outside, filepath.Join(dir, "references")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	_, err := skill.NewLoader().Load(dir)
	if !errors.Is(err, skill.ErrInvalidPackFile) {
		t.Fatalf("want ErrInvalidPackFile for symlinked references dir, got %v", err)
	}
}

func TestLoadReferenceDocRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	writePack(t, dir)
	pack, err := skill.NewLoader().Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pack.LoadReferenceDoc("../../etc/passwd"); !errors.Is(err, skill.ErrInvalidPackFile) {
		t.Fatalf("want ErrInvalidPackFile, got %v", err)
	}
}

func TestRegistryDuplicateWrapsSentinel(t *testing.T) {
	reg := skill.NewRegistry()
	if err := reg.Register(provider{manifest: validManifest()}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(provider{manifest: validManifest()}); !errors.Is(err, skill.ErrAlreadyRegistered) {
		t.Fatalf("want ErrAlreadyRegistered, got %v", err)
	}
}

func TestVerifierOutcomes(t *testing.T) {
	signed := validManifest()
	signed.Signature = &skill.Signature{Algorithm: "ed25519", Value: "sig", KeyID: "k"}
	if out, _ := (skill.WarnOnlyVerifier{}).Verify(&signed, ""); out.Status != skill.VerificationVerified {
		t.Fatalf("signed manifest should verify, got %+v", out)
	}
	unsigned := validManifest()
	if out, _ := (skill.WarnOnlyVerifier{}).Verify(&unsigned, ""); out.Status != skill.VerificationWarning {
		t.Fatalf("unsigned manifest should warn, got %+v", out)
	}
	placeholder := validManifest()
	placeholder.Signature = &skill.Signature{}
	if out, _ := (skill.WarnOnlyVerifier{}).Verify(&placeholder, ""); out.Status != skill.VerificationWarning {
		t.Fatalf("placeholder signature should still warn, got %+v", out)
	}
	if out, _ := (skill.DenyVerifier{}).Verify(&signed, ""); out.Status != skill.VerificationDenied {
		t.Fatalf("deny verifier should deny, got %+v", out)
	}
}

func TestLoaderLogsWarnings(t *testing.T) {
	dir := t.TempDir()
	head, _, _ := strings.Cut(manifest, "signature:")
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), head+"safety: read-only\n")
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# body")
	writeFile(t, filepath.Join(dir, "scripts", "run.sh"), "echo nope")

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	if _, err := skill.NewLoader(skill.WithLogger(logger)).Load(dir); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "unsigned skill manifest") {
		t.Fatalf("expected warning logged, got %q", buf.String())
	}
}

func TestLoadManifestRejectsNonRegularFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, skill.ManifestFileName), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := skill.LoadManifest(filepath.Join(dir, skill.ManifestFileName)); !errors.Is(err, skill.ErrInvalidPackFile) {
		t.Fatalf("want ErrInvalidPackFile, got %v", err)
	}
}

func TestLoaderRejectsMissingDeclaredScript(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# body")
	// Declared scripts/run.sh is never created.
	if _, err := skill.NewLoader().Load(dir); err == nil {
		t.Fatal("expected error for missing declared script")
	}
}

type errVerifier struct{}

func (errVerifier) Verify(*skill.Manifest, string) (skill.VerificationOutcome, error) {
	return skill.VerificationOutcome{}, errors.New("verifier boom")
}

func TestLoaderPropagatesVerifierError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# body")
	if _, err := skill.NewLoader(skill.WithVerifier(errVerifier{})).Load(dir); err == nil {
		t.Fatal("expected verifier error to propagate")
	}
	if _, _, err := skill.NewLoader(skill.WithVerifier(errVerifier{})).LoadMetadata(dir); err == nil {
		t.Fatal("expected LoadMetadata to propagate verifier error")
	}
}

func TestLoadRejectsMissingRoot(t *testing.T) {
	_, err := skill.NewLoader().Load(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error resolving a missing pack root")
	}
}

func TestLoaderRejectsEscapingSkillBody(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.md"), "secret")
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(dir, "SKILL.md")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := skill.NewLoader().Load(dir); !errors.Is(err, skill.ErrInvalidPackFile) {
		t.Fatalf("want ErrInvalidPackFile for escaping SKILL.md, got %v", err)
	}
}

func TestLoaderRejectsSymlinkedSkillBody(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	writeFile(t, filepath.Join(dir, "inside.md"), "body")
	// Symlink resolves within the root, so the escape check passes and the
	// symlink rejection in readBounded is what must fail the load.
	if err := os.Symlink("inside.md", filepath.Join(dir, "SKILL.md")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := skill.NewLoader().Load(dir); !errors.Is(err, skill.ErrInvalidPackFile) {
		t.Fatalf("want ErrInvalidPackFile for symlinked SKILL.md, got %v", err)
	}
}

func TestLoaderRejectsSymlinkedReferenceFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# body")
	writeFile(t, filepath.Join(dir, "scripts", "run.sh"), "echo nope")
	writeFile(t, filepath.Join(dir, "references", "real.md"), "ref")
	if err := os.Symlink("real.md", filepath.Join(dir, "references", "link.md")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := skill.NewLoader().Load(dir); !errors.Is(err, skill.ErrInvalidPackFile) {
		t.Fatalf("want ErrInvalidPackFile for symlinked reference file, got %v", err)
	}
}

func TestLoaderRejectsOversizedAsset(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# body")
	writeFile(t, filepath.Join(dir, "references", "big.md"), strings.Repeat("x", 32))
	loader := skill.NewLoader(skill.WithLimits(skill.Limits{Asset: 8}))
	if _, err := loader.Load(dir); !errors.Is(err, skill.ErrFileTooLarge) {
		t.Fatalf("want ErrFileTooLarge for oversized asset, got %v", err)
	}
}

func TestLoaderRejectsOversizedScript(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# body")
	writeFile(t, filepath.Join(dir, "scripts", "run.sh"), strings.Repeat("x", 32))
	loader := skill.NewLoader(skill.WithLimits(skill.Limits{Asset: 8}))
	if _, err := loader.Load(dir); !errors.Is(err, skill.ErrFileTooLarge) {
		t.Fatalf("want ErrFileTooLarge for oversized script, got %v", err)
	}
}

func TestLoaderRejectsAssetsExceedingTotal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# body")
	writeFile(t, filepath.Join(dir, "scripts", "run.sh"), "echo nope")
	writeFile(t, filepath.Join(dir, "references", "a.md"), strings.Repeat("x", 10))
	writeFile(t, filepath.Join(dir, "references", "b.md"), strings.Repeat("y", 10))
	// Each asset is under Asset but their sum exceeds AssetTotal.
	loader := skill.NewLoader(skill.WithLimits(skill.Limits{Asset: 100, AssetTotal: 15}))
	if _, err := loader.Load(dir); !errors.Is(err, skill.ErrAssetsTooLarge) {
		t.Fatalf("want ErrAssetsTooLarge, got %v", err)
	}
}

const multiAssetManifest = `schema_version: "1"
name: demo
version: 0.1.0
description: d
references:
  tools: []
  prompts: []
  resources: []
  mcp_servers: []
human_approval: []
scripts:
  - path: scripts/a.sh
    description: a
  - path: scripts/b.sh
    description: b
`

func TestLoaderSortsMultipleAssets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), multiAssetManifest)
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# body")
	writeFile(t, filepath.Join(dir, "references", "b.md"), "b")
	writeFile(t, filepath.Join(dir, "references", "a.md"), "a")
	writeFile(t, filepath.Join(dir, "scripts", "b.sh"), "b")
	writeFile(t, filepath.Join(dir, "scripts", "a.sh"), "a")
	pack, err := skill.NewLoader().Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.References) != 2 || pack.References[0].Path != "a.md" || pack.References[1].Path != "b.md" {
		t.Fatalf("references not sorted: %+v", pack.References)
	}
	if len(pack.Scripts) != 2 || pack.Scripts[0].Path != "scripts/a.sh" || pack.Scripts[1].Path != "scripts/b.sh" {
		t.Fatalf("scripts not sorted: %+v", pack.Scripts)
	}
}

func TestLoaderPartialLimitsFallBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# body")
	writeFile(t, filepath.Join(dir, "scripts", "run.sh"), "echo nope")
	writeFile(t, filepath.Join(dir, "references", "a.md"), "ref")
	// Only Body is set; Asset/AssetTotal/Manifest fall back to defaults, so a
	// normal-sized pack still loads.
	loader := skill.NewLoader(skill.WithLimits(skill.Limits{Body: 1 << 20}))
	if _, err := loader.Load(dir); err != nil {
		t.Fatalf("partial limits should keep defaults for unset fields: %v", err)
	}
}

func TestLoadReferenceDocRejectsMissingRoot(t *testing.T) {
	pack := skill.Pack{Root: filepath.Join(t.TempDir(), "does-not-exist")}
	if _, err := pack.LoadReferenceDoc("a.md"); err == nil {
		t.Fatal("expected error resolving a missing pack root")
	}
}

func TestLoadReferenceDocRejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.md"), "secret")
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(dir, "references", "link.md")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	pack := skill.Pack{Root: dir}
	if _, err := pack.LoadReferenceDoc("link.md"); !errors.Is(err, skill.ErrInvalidPackFile) {
		t.Fatalf("want ErrInvalidPackFile for escaping reference, got %v", err)
	}
}

func TestLoadReferenceDocRejectsEmptyName(t *testing.T) {
	pack := skill.Pack{Root: t.TempDir()}
	if _, err := pack.LoadReferenceDoc(""); !errors.Is(err, skill.ErrInvalidPackFile) {
		t.Fatalf("want ErrInvalidPackFile for empty name, got %v", err)
	}
}

type badStatusVerifier struct{}

func (badStatusVerifier) Verify(*skill.Manifest, string) (skill.VerificationOutcome, error) {
	return skill.VerificationOutcome{Status: skill.VerificationStatus(99)}, nil
}

func TestLoaderFailsClosedOnUnknownVerificationStatus(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# body")
	if _, err := skill.NewLoader(skill.WithVerifier(badStatusVerifier{})).Load(dir); !errors.Is(err, skill.ErrVerificationDenied) {
		t.Fatalf("want ErrVerificationDenied for unknown status, got %v", err)
	}
}

func TestLoaderMissingSkillBodyReturnsIOError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, skill.ManifestFileName), manifest)
	// SKILL.md is never written; the missing body is an IO failure, not a
	// pack-file policy violation.
	_, err := skill.NewLoader().Load(dir)
	if err == nil {
		t.Fatal("expected error for missing SKILL.md")
	}
	if errors.Is(err, skill.ErrInvalidPackFile) {
		t.Fatalf("missing body should pass through as an IO error, got ErrInvalidPackFile: %v", err)
	}
}

func TestLoaderPackRootIsCanonical(t *testing.T) {
	dir := t.TempDir()
	writePack(t, dir)
	canon, err := fs.Canonicalize(dir)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	pack, err := skill.NewLoader().Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if pack.Root != canon {
		t.Fatalf("pack root=%q, want canonical %q", pack.Root, canon)
	}
}
