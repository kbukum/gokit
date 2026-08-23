package process_test

import (
	"testing"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/process"
)

func TestResultSuccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		exitCode int
		want     bool
	}{
		{name: "zero", exitCode: 0, want: true},
		{name: "nonzero", exitCode: 2, want: false},
		{name: "killed", exitCode: -1, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := &process.Result{ExitCode: tt.exitCode}
			if got := r.Success(); got != tt.want {
				t.Fatalf("Success() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResultCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		result   process.Result
		wantErr  bool
		wantCode goerrors.ErrorCode
	}{
		{name: "success", result: process.Result{ExitCode: 0}, wantErr: false},
		{name: "canceled wins", result: process.Result{ExitCode: -1, Canceled: true}, wantErr: true, wantCode: goerrors.ErrCodeCanceled},
		{name: "timed out wins", result: process.Result{ExitCode: -1, TimedOut: true}, wantErr: true, wantCode: goerrors.ErrCodeTimeout},
		{name: "nonzero exit", result: process.Result{ExitCode: 3}, wantErr: true, wantCode: goerrors.ErrCodeInternal},
		{name: "killed", result: process.Result{ExitCode: -1}, wantErr: true, wantCode: goerrors.ErrCodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.result.Check()
			if tt.wantErr == (err == nil) {
				t.Fatalf("Check() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				return
			}
			appErr, ok := goerrors.AsAppError(err)
			if !ok {
				t.Fatalf("Check() = %T, want *errors.AppError", err)
			}
			if appErr.Code != tt.wantCode {
				t.Fatalf("Check() code = %s, want %s", appErr.Code, tt.wantCode)
			}
		})
	}
}

func TestResultCheckExitCodeDetail(t *testing.T) {
	t.Parallel()

	r := process.Result{ExitCode: 7}
	err := r.Check()
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatalf("Check() = %T, want *errors.AppError", err)
	}
	if appErr.Details["exit_code"] != 7 {
		t.Fatalf("exit_code detail = %v, want 7", appErr.Details["exit_code"])
	}
}
