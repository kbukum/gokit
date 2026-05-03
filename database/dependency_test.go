package database

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoreDoesNotImportDriverSDKs(t *testing.T) {
	t.Parallel()

	deniedPrefixes := []string{
		"gorm.io/driver/",
		"github.com/lib/pq",
		"github.com/go-sql-driver/mysql",
	}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch path {
			case "sqlite", "testutil":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, denied := range deniedPrefixes {
				if strings.HasPrefix(importPath, denied) {
					t.Fatalf("%s imports driver SDK %q; move it to an adapter module", path, importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk core imports: %v", err)
	}
}

func TestCoreModuleDoesNotRequireDriverSDKs(t *testing.T) {
	t.Parallel()

	deniedModules := []string{
		"gorm.io/driver/",
		"github.com/lib/pq",
		"github.com/go-sql-driver/mysql",
	}
	goMod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(goMod), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") || strings.Contains(line, "// indirect") {
			continue
		}
		for _, denied := range deniedModules {
			if strings.Contains(line, denied) {
				t.Fatalf("database/go.mod directly requires driver SDK %q; move it to an adapter module", line)
			}
		}
	}
}
