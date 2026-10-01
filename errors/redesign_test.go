package errors

import (
	"context"
	"testing"
)

func TestExplicitClassificationSurvivesContextCause(t *testing.T) {
	t.Parallel()
	want := Conflict("operation already committed").WithReason("ALREADY_COMMITTED").
		WithTraceID("trace-1").WithCause(context.DeadlineExceeded)
	if got := Normalize(want); got != want {
		t.Fatalf("explicit application outcome replaced: %#v", got)
	}
}

func TestFieldConstructorsUseViolations(t *testing.T) {
	t.Parallel()
	for _, err := range []*AppError{InvalidInput("name", "invalid"), MissingField("name"), InvalidFormat("name", "uuid")} {
		if len(err.Violations) != 1 || err.Violations[0].Field != "name" {
			t.Errorf("%s: missing structured field: %#v", err.Code, err.Violations)
		}
		if _, duplicate := err.Details["field"]; duplicate {
			t.Errorf("%s: duplicate field representation", err.Code)
		}
	}
}
