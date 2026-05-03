package storage

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoreDoesNotImportBackendSDKs(t *testing.T) {
	t.Parallel()

	deniedPrefixes := []string{"github.com/aws/aws-sdk-go-v2"}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == "s3" {
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
			for _, denied := range deniedPrefixes {
				if strings.HasPrefix(importPath, denied) {
					t.Fatalf("%s imports backend SDK %q; move it to an adapter module", path, importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk core imports: %v", err)
	}
}

func TestFactoryRegistryHasNoSideEffectProviders(t *testing.T) {
	t.Parallel()

	reg := NewFactoryRegistry()
	for _, provider := range []string{ProviderLocal, ProviderS3, ProviderSupabase} {
		if _, ok := reg.Get(provider); ok {
			t.Fatalf("provider %q registered without explicit adapter Register call", provider)
		}
	}
}
