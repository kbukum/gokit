package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kbukum/gokit/process"
)

// ArgvRecorder is an executable test double that records the exact argv values it receives.
type ArgvRecorder struct {
	Binary     string
	OutputPath string
}

// NewArgvRecorder creates an executable process fake that records argv values one per line.
func NewArgvRecorder(t *testing.T) *ArgvRecorder {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("argv recorder uses a POSIX executable script")
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, "argv-recorder")
	output := filepath.Join(dir, "argv.txt")
	script := "#!/bin/sh\n: > \"$GOKIT_PROCESS_ARGV_RECORDER_OUT\"\nfor arg in \"$@\"; do\n  printf '%s\\n' \"$arg\" >> \"$GOKIT_PROCESS_ARGV_RECORDER_OUT\"\ndone\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatalf("write argv recorder: %v", err)
	}
	return &ArgvRecorder{Binary: binary, OutputPath: output}
}

// Command returns a process command that invokes the recorder with args as literal argv values.
func (r *ArgvRecorder) Command(args ...string) process.Command {
	return process.Command{
		Binary:   r.Binary,
		Args:     args,
		Env:      []string{"GOKIT_PROCESS_ARGV_RECORDER_OUT=" + r.OutputPath},
		ScrubEnv: true,
	}
}

// Args reads the argv values recorded by the most recent run.
func (r *ArgvRecorder) Args(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(r.OutputPath)
	if err != nil {
		t.Fatalf("read argv recorder output: %v", err)
	}
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
