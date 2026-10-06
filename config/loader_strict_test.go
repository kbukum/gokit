package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/util"
)

func TestLoadConfigExplicitFilesMustExist(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	for name, opt := range map[string]LoaderOption{
		"config": WithConfigFile(missing + ".yml"),
		"env":    WithEnvFile(missing + ".env"),
	} {
		var cfg ServiceConfig
		err := LoadConfig("svc", &cfg, opt, WithoutDiscovery())
		if !errors.Is(err, ErrFileNotFound) {
			t.Fatalf("%s: err = %v, want ErrFileNotFound", name, err)
		}
	}
}

func TestLoadConfigExplicitProfileMustExist(t *testing.T) {
	dir := t.TempDir()
	var cfg ServiceConfig
	err := LoadConfig("svc", &cfg, WithProfile("missing"), WithProfileDir(dir), WithoutDiscovery())
	if !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("err = %v, want ErrFileNotFound", err)
	}
}

func TestLoadConfigRejectsInvalidProfileNames(t *testing.T) {
	for _, profile := range []string{"../escape", "a/b", "UPPER", "-lead", "a.b", "with space"} {
		var cfg ServiceConfig
		err := LoadConfig("svc", &cfg, WithProfile(profile), WithProfileDir(t.TempDir()), WithoutDiscovery())
		if !errors.Is(err, ErrInvalidProfile) {
			t.Fatalf("%q: err = %v, want ErrInvalidProfile", profile, err)
		}
	}
}

func TestLoadConfigImplicitProfileIsOptionalButValidated(t *testing.T) {
	t.Setenv("ENVIRONMENT", "staging")
	var cfg ServiceConfig
	if err := LoadConfig("svc", &cfg, WithProfile(""), WithProfileDir(t.TempDir()), WithoutDiscovery()); err != nil {
		t.Fatalf("missing implicit profile failed: %v", err)
	}
	t.Setenv("ENVIRONMENT", "../staging")
	if err := LoadConfig("svc", &cfg, WithProfile(""), WithProfileDir(t.TempDir()), WithoutDiscovery()); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("invalid implicit profile err = %v", err)
	}
}

func TestLoadConfigProfileDirLoadsOnlyThatDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := util.WriteFile(filepath.Join(dir, "local-dev.env"), []byte("VERSION=profile-dir\n")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VERSION", "")
	if err := os.Unsetenv("VERSION"); err != nil {
		t.Fatal(err)
	}
	var cfg ServiceConfig
	if err := LoadConfig("svc", &cfg, WithProfile("local-dev"), WithProfileDir(dir), WithoutDiscovery()); err != nil {
		t.Fatal(err)
	}
	if cfg.Version != "profile-dir" {
		t.Fatalf("Version = %q", cfg.Version)
	}
}

func TestWithoutDiscoverySkipsSearch(t *testing.T) {
	fs := &mockFS{files: map[string]bool{"./config.yml": true, ".env": true, "./config/profiles/dev.env": true}}
	r := &Resolver{FileSystem: fs}
	files, err := r.ResolveFiles("svc", LoaderConfig{DisableDiscovery: true})
	if err != nil || files != (ResolvedFiles{}) {
		t.Fatalf("files = %+v, err = %v", files, err)
	}
}

// vanishingFS reports each path present once, modeling a file removed between resolution and load.
type vanishingFS struct {
	RealFileSystem
	seen map[string]bool
}

func (f *vanishingFS) Exists(path string) (bool, error) {
	if f.seen[path] {
		return false, nil
	}
	f.seen[path] = true
	return true, nil
}

func TestLoadConfigResolvedFileRemovedBeforeLoadFails(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	for name, opt := range map[string]LoaderOption{
		"config": WithConfigFile(missing + ".yml"),
		"env":    WithEnvFile(missing + ".env"),
	} {
		var cfg ServiceConfig
		err := LoadConfig("svc", &cfg, opt, WithoutDiscovery(), WithFileSystem(&vanishingFS{seen: map[string]bool{}}))
		if !errors.Is(err, ErrFileNotFound) {
			t.Fatalf("%s: err = %v, want ErrFileNotFound", name, err)
		}
	}
}

// failingFS reports a stat failure that is not "not found".
type failingFS struct{ RealFileSystem }

var errStatDenied = errors.New("stat denied")

func (*failingFS) Exists(string) (bool, error) { return false, errStatDenied }

func TestResolveFilesPreservesProbeFailures(t *testing.T) {
	for name, opts := range map[string]LoaderConfig{
		"explicit":  {ConfigFile: "config.yml"},
		"discovery": {},
		"profile":   {ProfileEnabled: true, Profile: "dev", DisableDiscovery: true, ProfileDir: "profiles"},
	} {
		_, err := (&Resolver{FileSystem: &failingFS{}}).ResolveFiles("svc", opts)
		if !errors.Is(err, errStatDenied) || errors.Is(err, ErrFileNotFound) {
			t.Fatalf("%s: err = %v, want stat failure", name, err)
		}
	}
}

func TestRealFileSystemExistsReportsStatFailures(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	ok, err := (&RealFileSystem{}).Exists(filepath.Join(dir, "config.yml"))
	if ok || err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Exists in unreadable dir = %v, %v; want permission error", ok, err)
	}
}
