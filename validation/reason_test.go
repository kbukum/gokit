package validation

import (
	"testing"

	"github.com/kbukum/gokit/errors"
)

func TestSemanticReasons(t *testing.T) {
	t.Parallel()
	for rule, want := range map[string]errors.ViolationReason{
		"required":       errors.ViolationRequired,
		"string.email":   errors.ViolationInvalidFormat,
		"string.min_len": errors.ViolationOutOfRange,
		"custom.rule":    errors.ViolationInvalidValue,
	} {
		if got := ReasonForRule(rule); got != want {
			t.Errorf("%s: got %s, want %s", rule, got, want)
		}
	}
}

func TestEvaluationFailuresAreInternal(t *testing.T) {
	t.Parallel()
	for _, got := range []*errors.AppError{
		NewStructValidator().Validate(42),
		New().Pattern("field", "input", "[").Validate(),
	} {
		if got == nil || got.Code != errors.ErrCodeInternal || got.Cause == nil || len(got.Violations) != 0 {
			t.Fatalf("evaluation failure attributed to input: %#v", got)
		}
	}
}
