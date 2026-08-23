package process_test

import (
	stderrors "errors"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"testing"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/process"
)

func TestSpawnError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode goerrors.ErrorCode
	}{
		{name: "not found", err: exec.ErrNotFound, wantCode: goerrors.ErrCodeNotFound},
		{name: "os not exist", err: &fs.PathError{Op: "fork/exec", Err: os.ErrNotExist}, wantCode: goerrors.ErrCodeNotFound},
		{name: "permission", err: &fs.PathError{Op: "fork/exec", Err: os.ErrPermission}, wantCode: goerrors.ErrCodeForbidden},
		{name: "other", err: stderrors.New("boom"), wantCode: goerrors.ErrCodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := process.SpawnError("process: start foo", tt.err)
			appErr, ok := goerrors.AsAppError(err)
			if !ok {
				t.Fatalf("SpawnError() = %T, want *errors.AppError", err)
			}
			if appErr.Code != tt.wantCode {
				t.Fatalf("code = %s, want %s", appErr.Code, tt.wantCode)
			}
			if !strings.HasPrefix(appErr.Message, "process: start foo: ") {
				t.Fatalf("message = %q, want context prefix", appErr.Message)
			}
			if !stderrors.Is(err, tt.err) {
				t.Fatalf("SpawnError() should preserve cause %v", tt.err)
			}
		})
	}
}

func TestSpawnErrorFromRun(t *testing.T) {
	t.Parallel()

	_, err := process.Run(t.Context(), process.Command{Binary: "nonexistent_binary_xyz_99999"})
	if err == nil {
		t.Fatal("expected error for non-existent binary")
	}
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatalf("Run() = %T, want *errors.AppError", err)
	}
	if appErr.Code != goerrors.ErrCodeNotFound {
		t.Fatalf("code = %s, want NOT_FOUND", appErr.Code)
	}
}
