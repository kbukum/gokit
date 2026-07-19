package cache

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// deniedCacheSDKs are concrete cache-backend client SDKs that must never appear in the
// backend-agnostic cache core; live-backend behavior belongs in the adapter sub-modules
// (e.g. cache/redis), whose own go.mod legitimately owns the SDK.
var deniedCacheSDKs = []string{
	"github.com/redis/go-redis",
	"github.com/go-redis/redis",
	"github.com/bradfitz/gomemcache",
}

func TestCoreDoesNotImportCacheSDKs(t *testing.T) {
	t.Parallel()

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch path {
			case "redis", "testutil":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, denied := range deniedCacheSDKs {
				if strings.HasPrefix(importPath, denied) {
					t.Fatalf("%s imports cache SDK %q; move it to an adapter module", path, importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk core imports: %v", err)
	}
}

func TestCoreModuleDoesNotRequireCacheSDKs(t *testing.T) {
	t.Parallel()

	goMod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(goMod), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") || strings.Contains(line, "// indirect") {
			continue
		}
		for _, denied := range deniedCacheSDKs {
			if strings.Contains(line, denied) {
				t.Fatalf("cache/go.mod directly requires cache SDK %q; move it to an adapter module", line)
			}
		}
	}
}
