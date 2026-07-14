package skill_test

import (
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/skill"
	"github.com/kbukum/gokit/util"
)

// FuzzParseManifest exercises the manifest decoder and validator against
// arbitrary bytes. Parsing must never panic and must fail closed: any returned
// manifest must pass validation.
func FuzzParseManifest(f *testing.F) {
	f.Add([]byte(manifest))
	f.Add([]byte("name: [bad"))
	f.Add([]byte(""))
	f.Add([]byte("schema_version: \"1\"\nname: x\nversion: 0.1.0\ndescription: d\nreferences: {}\n"))
	f.Add([]byte("safety: destructive\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := skill.ParseManifest(data)
		if err != nil {
			if m != nil {
				t.Fatalf("error returned with non-nil manifest: %v", err)
			}
			return
		}
		if err := skill.Validate(m); err != nil {
			t.Fatalf("ParseManifest returned an invalid manifest: %v", err)
		}
	})
}

// FuzzLoad exercises the full activation path (manifest + body + assets) against
// arbitrary manifest and body bytes written into a real pack directory. Loading
// must never panic; failures must be errors, and any returned pack must carry
// the parsed manifest.
func FuzzLoad(f *testing.F) {
	f.Add([]byte(manifest), []byte("# body"))
	f.Add([]byte("name: [bad"), []byte(""))
	f.Add([]byte(""), []byte("---\ntitle: x\n---\nbody"))

	f.Fuzz(func(t *testing.T, manifestBytes, body []byte) {
		dir := t.TempDir()
		if err := util.WriteFile(filepath.Join(dir, skill.ManifestFileName), manifestBytes); err != nil {
			t.Skip()
		}
		if err := util.WriteFile(filepath.Join(dir, skill.SkillMarkdownFileName), body); err != nil {
			t.Skip()
		}
		pack, err := skill.NewLoader().Load(dir)
		if err != nil {
			if pack != nil {
				t.Fatalf("error returned with non-nil pack: %v", err)
			}
			return
		}
		if pack.Manifest.Name == "" {
			t.Fatalf("loaded pack has empty manifest name")
		}
	})
}
