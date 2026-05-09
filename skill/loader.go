package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const SkillMarkdownFileName = "SKILL.md"

type Asset struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type Pack struct {
	Root       string
	Manifest   Manifest
	SkillBody  string
	References []Asset
	Scripts    []Asset
}

type Loader struct{}

func NewLoader() *Loader { return &Loader{} }

func (l *Loader) Load(root string) (*Pack, error) {
	manifest, err := LoadManifest(filepath.Join(root, ManifestFileName))
	if err != nil {
		return nil, err
	}
	mdPath := filepath.Join(root, SkillMarkdownFileName)
	if symErr := rejectSymlink(mdPath); symErr != nil {
		return nil, symErr
	}
	bodyBytes, err := os.ReadFile(mdPath)
	if err != nil {
		return nil, fmt.Errorf("skill: read %s: %w", SkillMarkdownFileName, err)
	}
	refs, err := enumerateAssets(root, "references")
	if err != nil {
		return nil, err
	}
	scripts, err := enumerateDeclaredScripts(root, manifest.Scripts)
	if err != nil {
		return nil, err
	}
	return &Pack{Root: root, Manifest: *manifest, SkillBody: splitFrontmatter(string(bodyBytes)), References: refs, Scripts: scripts}, nil
}

func (p *Pack) LoadReferenceDoc(name string) ([]byte, error) {
	clean := filepath.Clean(name)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return nil, fmt.Errorf("skill: invalid reference path %q", name)
	}
	full := filepath.Join(p.Root, "references", clean)
	if err := rejectSymlink(full); err != nil {
		return nil, err
	}
	return os.ReadFile(full)
}

func (p *Pack) ListScripts() []Asset {
	out := make([]Asset, len(p.Scripts))
	copy(out, p.Scripts)
	return out
}

// rejectSymlink returns an error if the path is a symbolic link,
// preventing symlink-escape attacks in skill packs.
func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("skill: symlinked files are not allowed: %s", path)
	}
	return nil
}

func enumerateDeclaredScripts(root string, scripts []Script) ([]Asset, error) {
	assets := make([]Asset, 0, len(scripts))
	for _, script := range scripts {
		clean := filepath.Clean(script.Path)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return nil, fmt.Errorf("skill: invalid script path %q", script.Path)
		}
		full := filepath.Join(root, clean)
		if err := rejectSymlink(full); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		assets = append(assets, Asset{Path: filepath.ToSlash(clean), SHA256: hex.EncodeToString(sum[:])})
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Path < assets[j].Path })
	return assets, nil
}

func enumerateAssets(root, dir string) ([]Asset, error) {
	base := filepath.Join(root, filepath.Clean(dir))
	if st, err := os.Stat(base); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	} else if !st.IsDir() {
		return nil, fmt.Errorf("skill: %s is not a directory", dir)
	}
	var assets []Asset
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill: symlinked assets are not allowed: %s", path)
		}
		//nolint:gosec // WalkDir is scoped to base and symlinks are rejected before reading; Go 1.25 lacks os.Root-scoped reads.
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		assets = append(assets, Asset{Path: filepath.ToSlash(rel), SHA256: hex.EncodeToString(sum[:])})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Path < assets[j].Path })
	return assets, nil
}

func splitFrontmatter(body string) string {
	if strings.HasPrefix(body, "---\n") {
		if idx := strings.Index(body[4:], "\n---\n"); idx >= 0 {
			return strings.TrimSpace(body[idx+9:])
		}
	}
	return strings.TrimSpace(body)
}
