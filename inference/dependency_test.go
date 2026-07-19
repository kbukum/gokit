package inference

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// deniedInferenceSDKs are concrete model-serving/vendor client SDKs that must never appear in the
// backend-agnostic inference core; adapters speak to model servers over HTTP. Any adapter needing a
// vendor SDK belongs in its own sub-module (e.g. inference/tgi, inference/vllm, inference/triton),
// whose own go.mod would legitimately own the dependency.
var deniedInferenceSDKs = []string{
	"github.com/sashabaranov/go-openai",
	"github.com/nvidia",
	"github.com/yalue/onnxruntime_go",
}

func TestCoreDoesNotImportInferenceSDKs(t *testing.T) {
	t.Parallel()

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch path {
			case "tgi", "vllm", "triton", "testutil":
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
			for _, denied := range deniedInferenceSDKs {
				if strings.HasPrefix(importPath, denied) {
					t.Fatalf("%s imports inference SDK %q; move it to an adapter module", path, importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk core imports: %v", err)
	}
}

func TestCoreModuleDoesNotRequireInferenceSDKs(t *testing.T) {
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
		for _, denied := range deniedInferenceSDKs {
			if strings.Contains(line, denied) {
				t.Fatalf("inference/go.mod directly requires inference SDK %q; move it to an adapter module", line)
			}
		}
	}
}
