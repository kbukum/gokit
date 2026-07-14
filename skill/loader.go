package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/fs"
)

const SkillMarkdownFileName = "SKILL.md"

// Size limits for untrusted skill-pack files. Reads fail closed once exceeded.
const (
	// MaxBodyBytes bounds the SKILL.md body size.
	MaxBodyBytes = 4 << 20 // 4 MiB
	// MaxAssetBytes bounds a single inert asset size.
	MaxAssetBytes = 16 << 20 // 16 MiB
	// MaxAssetTotalBytes bounds the aggregate size of all inert assets.
	MaxAssetTotalBytes = 64 << 20 // 64 MiB
)

type Asset struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type Pack struct {
	Root                 string
	Manifest             Manifest
	SkillBody            string
	References           []Asset
	Scripts              []Asset
	VerificationWarnings []string

	limits Limits
}

// LoaderOption configures a Loader.
type LoaderOption func(*Loader)

// WithVerifier injects the manifest verifier consulted at load time. When
// omitted the loader uses WarnOnlyVerifier.
func WithVerifier(v Verifier) LoaderOption {
	return func(l *Loader) {
		if v != nil {
			l.verifier = v
		}
	}
}

// WithLogger injects the logger used to surface non-fatal verification warnings.
func WithLogger(logger *slog.Logger) LoaderOption {
	return func(l *Loader) { l.logger = logger }
}

// WithLimits injects the size limits enforced on untrusted pack files. Any
// non-positive field falls back to its DefaultLimits value.
func WithLimits(limits Limits) LoaderOption {
	return func(l *Loader) { l.limits = limits.withDefaults() }
}

// Loader loads and activates skill packs from the filesystem, enforcing size
// limits, rejecting symlinks and escaping paths, and running an injected
// verifier before a pack is returned.
type Loader struct {
	verifier Verifier
	logger   *slog.Logger
	limits   Limits
}

// NewLoader builds a Loader. Without options it verifies with WarnOnlyVerifier
// and enforces DefaultLimits.
func NewLoader(opts ...LoaderOption) *Loader {
	l := &Loader{verifier: WarnOnlyVerifier{}, limits: DefaultLimits()}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// LoadMetadata parses, validates, and verifies only the manifest. It returns
// the manifest and any non-fatal verification warnings, and fails closed when
// verification is denied.
func (l *Loader) LoadMetadata(root string) (*Manifest, []string, error) {
	manifest, err := loadManifest(filepath.Join(root, ManifestFileName), l.limits.Manifest)
	if err != nil {
		return nil, nil, err
	}
	outcome, err := l.verifier.Verify(manifest, root)
	if err != nil {
		return nil, nil, err
	}
	switch outcome.Status {
	case VerificationVerified:
		return manifest, nil, nil
	case VerificationWarning:
		l.logWarnings(root, outcome.Warnings)
		return manifest, outcome.Warnings, nil
	case VerificationDenied:
		return nil, nil, fmt.Errorf("%w: %s", ErrVerificationDenied, outcome.Reason)
	default:
		return nil, nil, fmt.Errorf("%w: unknown verification status %d", ErrVerificationDenied, outcome.Status)
	}
}

// Load activates a skill pack: it loads verified metadata, the SKILL.md body,
// and the inert reference and script asset inventory.
func (l *Loader) Load(root string) (*Pack, error) {
	canonRoot, err := fs.Canonicalize(root)
	if err != nil {
		return nil, err
	}
	manifest, warnings, err := l.LoadMetadata(root)
	if err != nil {
		return nil, err
	}

	mdPath := filepath.Join(root, SkillMarkdownFileName)
	if escErr := confineToRoot(canonRoot, mdPath); escErr != nil {
		return nil, escErr
	}
	bodyBytes, err := readBounded(mdPath, l.limits.Body)
	if err != nil {
		return nil, fmt.Errorf("skill: read %s: %w", SkillMarkdownFileName, err)
	}
	if !utf8.Valid(bodyBytes) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidUTF8, mdPath)
	}

	var total int64
	refs, err := enumerateAssets(canonRoot, root, "references", &total, l.limits)
	if err != nil {
		return nil, err
	}
	scripts, err := enumerateDeclaredScripts(canonRoot, root, manifest.Scripts, &total, l.limits)
	if err != nil {
		return nil, err
	}
	return &Pack{
		Root:                 root,
		Manifest:             *manifest,
		SkillBody:            splitFrontmatter(string(bodyBytes)),
		References:           refs,
		Scripts:              scripts,
		VerificationWarnings: warnings,
		limits:               l.limits,
	}, nil
}

func (l *Loader) logWarnings(root string, warnings []string) {
	if l.logger == nil {
		return
	}
	for _, warning := range warnings {
		l.logger.Warn("skill verification warning", "root", root, "warning", warning)
	}
}

func (p *Pack) LoadReferenceDoc(name string) ([]byte, error) {
	if name == "" || fs.ValidateRelativePath(name) != nil {
		return nil, fmt.Errorf("%w: invalid reference path %q", ErrInvalidPackFile, name)
	}
	canonRoot, err := fs.Canonicalize(p.Root)
	if err != nil {
		return nil, err
	}
	full := filepath.Join(p.Root, "references", filepath.Clean(name))
	if err := confineToRoot(canonRoot, full); err != nil {
		return nil, err
	}
	return readBounded(full, p.limits.withDefaults().Asset)
}

func (p *Pack) ListScripts() []Asset {
	out := make([]Asset, len(p.Scripts))
	copy(out, p.Scripts)
	return out
}

// rejectSymlink returns an error if the path is a symbolic link, preventing
// symlink-escape attacks in skill packs.
func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: symlinked files are not allowed: %s", ErrInvalidPackFile, path)
	}
	return nil
}

// readBounded reads a regular file after rejecting symlinks, failing closed when
// the file is not regular or exceeds maxBytes. It delegates the bounded read to
// the fs owner and maps its rejections onto the pack-file error taxonomy.
func readBounded(path string, maxBytes int64) ([]byte, error) {
	if err := rejectSymlink(path); err != nil {
		return nil, err
	}
	data, err := fs.ReadFileLimit(path, maxBytes)
	switch {
	case errors.Is(err, fs.ErrFileTooLarge):
		return nil, fmt.Errorf("%w: %s exceeds %d bytes", ErrFileTooLarge, path, maxBytes)
	case errors.Is(err, fs.ErrNotRegularFile):
		return nil, fmt.Errorf("%w: %s: expected regular file", ErrInvalidPackFile, path)
	case err != nil:
		return nil, err
	}
	return data, nil
}

func hashAsset(path string, total *int64, limits Limits) (string, error) {
	remaining := limits.AssetTotal - *total
	if remaining < 0 {
		remaining = 0
	}
	// Bound this read by the smaller of the per-asset and remaining-total
	// budgets so an oversized asset never fully materializes in memory before
	// the aggregate limit is enforced.
	limit := limits.Asset
	overTotal := false
	if remaining < limit {
		limit = remaining
		overTotal = true
	}
	data, err := readBounded(path, limit)
	if err != nil {
		if overTotal && errors.Is(err, ErrFileTooLarge) {
			return "", fmt.Errorf("%w: reading %s", ErrAssetsTooLarge, path)
		}
		return "", err
	}
	*total += int64(len(data))
	if *total > limits.AssetTotal {
		return "", fmt.Errorf("%w: reading %s (%d bytes)", ErrAssetsTooLarge, path, *total)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func enumerateDeclaredScripts(canonRoot, root string, scripts []Script, total *int64, limits Limits) ([]Asset, error) {
	assets := make([]Asset, 0, len(scripts))
	for _, script := range scripts {
		if err := fs.ValidateRelativePath(script.Path); err != nil {
			return nil, fmt.Errorf("%w: invalid script path %q", ErrInvalidPackFile, script.Path)
		}
		clean := filepath.Clean(script.Path)
		full := filepath.Join(root, clean)
		if err := confineToRoot(canonRoot, full); err != nil {
			return nil, err
		}
		digest, err := hashAsset(full, total, limits)
		if err != nil {
			return nil, err
		}
		assets = append(assets, Asset{Path: filepath.ToSlash(clean), SHA256: digest})
	}
	slices.SortFunc(assets, func(a, b Asset) int { return strings.Compare(a.Path, b.Path) })
	return assets, nil
}

func enumerateAssets(canonRoot, root, dir string, total *int64, limits Limits) ([]Asset, error) {
	base := filepath.Join(root, filepath.Clean(dir))
	info, err := os.Lstat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: symlinked assets are not allowed: %s", ErrInvalidPackFile, base)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrInvalidPackFile, dir)
	}
	if escErr := confineToRoot(canonRoot, base); escErr != nil {
		return nil, escErr
	}
	var assets []Asset
	err = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlinked assets are not allowed: %s", ErrInvalidPackFile, path)
		}
		if d.IsDir() {
			return nil
		}
		if escErr := confineToRoot(canonRoot, path); escErr != nil {
			return escErr
		}
		digest, err := hashAsset(path, total, limits)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		assets = append(assets, Asset{Path: filepath.ToSlash(rel), SHA256: digest})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(assets, func(a, b Asset) int { return strings.Compare(a.Path, b.Path) })
	return assets, nil
}

// confineToRoot rejects path when, after resolving symlinks, it escapes
// canonRoot. It reuses the fs owner's confinement and maps only its policy
// rejection (an escape) onto ErrInvalidPackFile; low-level IO failures (missing
// or unreadable paths) are returned as-is per the package error taxonomy.
func confineToRoot(canonRoot, path string) error {
	_, err := fs.ConfineExistingPath(canonRoot, path)
	if err == nil {
		return nil
	}
	if appErr, ok := apperrors.AsAppError(err); ok && appErr.Code == apperrors.ErrCodeInvalidInput {
		return fmt.Errorf("%w: %w", ErrInvalidPackFile, err)
	}
	return err
}

func splitFrontmatter(body string) string {
	if strings.HasPrefix(body, "---\n") {
		if idx := strings.Index(body[4:], "\n---\n"); idx >= 0 {
			return strings.TrimSpace(body[idx+9:])
		}
	}
	return strings.TrimSpace(body)
}
