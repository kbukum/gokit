package vectorstore

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// deniedVectorStoreSDKs are concrete vector-database client SDKs that must never appear in the
// backend-agnostic vectorstore core; live-backend behavior belongs in the adapter sub-modules
// (e.g. vectorstore/qdrant), whose own go.mod legitimately owns the SDK.
var deniedVectorStoreSDKs = []string{
	"github.com/qdrant/go-client",
	"github.com/weaviate/weaviate",
	"github.com/pinecone-io/go-pinecone",
	"github.com/milvus-io/milvus-sdk-go",
}

func TestCoreDoesNotImportVectorStoreSDKs(t *testing.T) {
	t.Parallel()

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch path {
			case "qdrant", "testutil":
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
			for _, denied := range deniedVectorStoreSDKs {
				if strings.HasPrefix(importPath, denied) {
					t.Fatalf("%s imports vector-store SDK %q; move it to an adapter module", path, importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk core imports: %v", err)
	}
}

func TestCoreModuleDoesNotRequireVectorStoreSDKs(t *testing.T) {
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
		for _, denied := range deniedVectorStoreSDKs {
			if strings.Contains(line, denied) {
				t.Fatalf("vectorstore/go.mod directly requires vector-store SDK %q; move it to an adapter module", line)
			}
		}
	}
}
