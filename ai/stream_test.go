package ai_test

import (
	"errors"
	"testing"

	"github.com/kbukum/gokit/ai"
)

func TestStreamErrorWrapsCause(t *testing.T) {
	t.Parallel()
	cause := errors.New("upstream reset")
	streamErr := ai.Error{Err: cause}

	if streamErr.Error() != "upstream reset" {
		t.Fatalf("Error() = %q, want %q", streamErr.Error(), "upstream reset")
	}
	if !errors.Is(streamErr, cause) {
		t.Fatal("errors.Is must unwrap the terminal stream error to its cause")
	}
	if got := streamErr.Unwrap(); !errors.Is(got, cause) {
		t.Fatalf("Unwrap() = %v, want %v", got, cause)
	}
}

func TestStreamErrorNilCause(t *testing.T) {
	t.Parallel()
	var streamErr ai.Error
	if streamErr.Error() != "ai: unknown stream error" {
		t.Fatalf("nil-cause Error() = %q", streamErr.Error())
	}
	if streamErr.Unwrap() != nil {
		t.Fatal("nil-cause Unwrap() must be nil")
	}
}

// TestStreamEventVariantsSeal proves each concrete stream event satisfies the
// sealed StreamEvent interface and carries its payload.
func TestStreamEventVariantsSeal(t *testing.T) {
	t.Parallel()
	events := []ai.StreamEvent{
		ai.TextDelta{Index: 1, Text: "hi"},
		ai.UsageDelta{InputTokens: 2, OutputTokens: 3},
		ai.Error{Err: errors.New("x")},
	}
	if td, ok := events[0].(ai.TextDelta); !ok || td.Text != "hi" {
		t.Fatalf("event[0] = %#v", events[0])
	}
	if ud, ok := events[1].(ai.UsageDelta); !ok || ud.OutputTokens != 3 {
		t.Fatalf("event[1] = %#v", events[1])
	}
}
