package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestFailurePreservesClassificationWithoutFormattingCause(t *testing.T) {
	t.Parallel()
	cause := errors.New("synthetic confidential cause")
	err := Failure(cause)
	for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
		if strings.Contains(fmt.Sprintf(format, err), cause.Error()) {
			t.Fatal("database boundary formatted a private cause")
		}
	}
	var app *apperrors.AppError
	if !errors.Is(err, cause) || !errors.As(err, &app) || app.Reason != "DATABASE_FAILURE" {
		t.Fatal("safe formatting lost typed error or cause")
	}
}

func TestFailureNormalizesToItsSafeClassification(t *testing.T) {
	t.Parallel()
	private := apperrors.InvalidInput("password", "synthetic confidential detail")
	for name, tc := range map[string]struct {
		cause error
		code  apperrors.ErrorCode
	}{
		"application cause": {private, apperrors.ErrCodeInternal},
		"wrapped cause":     {fmt.Errorf("driver: %w", private), apperrors.ErrCodeInternal},
		"deadline":          {fmt.Errorf("dial: %w", context.DeadlineExceeded), apperrors.ErrCodeTimeout},
		"canceled":          {context.Canceled, apperrors.ErrCodeCanceled},
	} {
		got := apperrors.Normalize(Failure(tc.cause))
		if got.Code != tc.code || got.Cause != nil || strings.Contains(got.Message, "confidential") {
			t.Errorf("%s: normalized to %v %q (cause %v)", name, got.Code, got.Message, got.Cause)
		}
		if !errors.Is(Failure(tc.cause), tc.cause) {
			t.Errorf("%s: cause lost", name)
		}
	}
}
