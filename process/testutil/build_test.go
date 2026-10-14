package testutil_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kbukum/gokit/process"
	"github.com/kbukum/gokit/process/testutil"
)

func TestBuildGoValidatesInputs(t *testing.T) {
	t.Parallel()
	for _, cfg := range []testutil.GoBuild{
		{},
		{Package: "-o"},
		{Package: "./testdata/program"},
		{Package: "-o", Output: "program"},
		{Package: "./testdata/program", Output: "program", CoverPackages: []string{"./..."}},
		{Package: "./testdata/program", Output: "program", Coverage: true, CoverPackages: []string{"-flag"}},
	} {
		if err := testutil.BuildGo(t.Context(), cfg); err == nil {
			t.Fatal("incomplete build accepted")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := testutil.BuildGo(ctx, testutil.GoBuild{
		Package: "./testdata/program", Output: filepath.Join(t.TempDir(), "program"),
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation cause lost: %v", err)
	}
}

func TestBuildGoInstrumentsActualChild(t *testing.T) {
	t.Parallel()
	binary := filepath.Join(t.TempDir(), "program")
	if err := testutil.BuildGo(t.Context(), testutil.GoBuild{
		Package: "./testdata/program", Output: binary, Race: true, Coverage: true,
		CoverPackages: []string{"github.com/kbukum/gokit/process/testutil/testdata/program"},
	}); err != nil {
		t.Fatal(err)
	}
	coverage := t.TempDir()
	result, err := process.Run(t.Context(), process.Command{
		Binary: binary, MaxOutputBytes: 4096, Env: map[string]string{"GOCOVERDIR": coverage},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.Stdout), "-race=true") ||
		!strings.Contains(string(result.Stdout), "-cover=true") {
		t.Fatal("actual child lacks declared instrumentation")
	}
	files, err := os.ReadDir(coverage)
	if err != nil {
		t.Fatal(err)
	}
	var metadata, counters bool
	for _, file := range files {
		metadata = metadata || strings.HasPrefix(file.Name(), "covmeta.")
		counters = counters || strings.HasPrefix(file.Name(), "covcounters.")
	}
	if !metadata || !counters {
		t.Fatal("actual child did not emit coverage metadata and counters")
	}
	if err := testutil.BuildGo(t.Context(), testutil.GoBuild{
		Package: "./testdata/not-present", Output: binary,
	}); err == nil {
		t.Fatal("invalid package produced a successful build")
	}
}

func TestBuildGoReportsCompilerDiagnostics(t *testing.T) {
	t.Parallel()
	err := testutil.BuildGo(t.Context(), testutil.GoBuild{Package: "./testdata/broken", Output: filepath.Join(t.TempDir(), "broken")})
	if err == nil || !strings.Contains(err.Error(), "testdata/broken/main.go") || !strings.Contains(err.Error(), "not a number") {
		t.Fatalf("build failure omitted its compiler cause: %v", err)
	}
}

func TestBuildGoCanMatchParentRaceInstrumentation(t *testing.T) {
	t.Parallel()
	binary := filepath.Join(t.TempDir(), "program")
	if err := testutil.BuildGo(t.Context(), testutil.GoBuild{
		Package: "./testdata/program", Output: binary, Race: testutil.RaceEnabled,
	}); err != nil {
		t.Fatal(err)
	}
	result, err := process.Run(t.Context(), process.Command{Binary: binary, MaxOutputBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Check(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Contains(result.StdoutString(), "-race=true"); got != testutil.RaceEnabled {
		t.Fatalf("child race=%t, parent race=%t", got, testutil.RaceEnabled)
	}
	if strings.Contains(result.StdoutString(), "-cover=true") {
		t.Fatal("race selection unexpectedly enabled coverage")
	}
}
