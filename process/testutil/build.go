package testutil

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/process"
)

// GoBuild declares one child program and its instrumentation independently of the parent test.
type GoBuild struct {
	Package       string
	Output        string
	Dir           string
	Race          bool
	Coverage      bool
	CoverPackages []string
}

// BuildGo compiles a child with an isolated module selection, a five-minute ceiling and at most 1 MiB of diagnostics per stream. The caller owns Output and its cleanup.
func BuildGo(ctx context.Context, cfg GoBuild) error {
	if !buildPackage(cfg.Package) || cfg.Output == "" {
		return apperrors.InvalidInput("Go build", "One package and an output path are required")
	}
	if len(cfg.CoverPackages) != 0 && !cfg.Coverage {
		return apperrors.InvalidInput("Go build", "Coverage packages require coverage instrumentation")
	}
	for _, pkg := range cfg.CoverPackages {
		if !buildPackage(pkg) || strings.Contains(pkg, ",") {
			return apperrors.InvalidInput("Go build", "Invalid coverage package")
		}
	}
	if err := apperrors.FromContext(ctx, "Go build"); err != nil {
		return err
	}
	args := []string{"build"}
	if cfg.Race {
		args = append(args, "-race")
	}
	if cfg.Coverage {
		args = append(args, "-cover", "-covermode=atomic")
		if len(cfg.CoverPackages) != 0 {
			args = append(args, "-coverpkg="+strings.Join(cfg.CoverPackages, ","))
		}
	}
	args = append(args, "-o", cfg.Output, cfg.Package)
	bounded, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	result, err := process.Run(bounded, process.Command{
		Binary: "go", Args: args, Dir: cfg.Dir,
		Env:            map[string]string{"GOWORK": "off", "GOFLAGS": ""},
		MaxOutputBytes: 1 << 20,
	})
	if result != nil && (result.StdoutTruncated || result.StderrTruncated) {
		err = errors.Join(err, errors.New("go build diagnostics exceeded 1 MiB per stream"))
	}
	if err == nil {
		err = result.Check()
	}
	if err != nil {
		cause := errors.Join(err, bounded.Err())
		failure := apperrors.Normalize(err).WithReason("GO_BUILD_FAILED")
		if result != nil {
			// The compiler's bounded output is the actionable cause of a failed build.
			cause = fmt.Errorf("%w\n%s%s", cause, result.StdoutString(), result.StderrString())
			failure = failure.WithDetail("stdout", result.StdoutString()).WithDetail("stderr", result.StderrString())
		}
		return failure.WithCause(cause)
	}
	return nil
}

func buildPackage(pkg string) bool {
	return pkg != "" && !strings.HasPrefix(pkg, "-") && !strings.ContainsAny(pkg, " \t\r\n\x00")
}
