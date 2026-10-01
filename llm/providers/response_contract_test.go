package providers_test

import (
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/llm/providers/anthropic"
	"github.com/kbukum/gokit/llm/providers/gemini"
	"github.com/kbukum/gokit/llm/providers/openai"
)

func TestUpstreamParsingFailuresAreNotCallerValidation(t *testing.T) {
	t.Parallel()
	for name, dialect := range map[string]llm.Dialect{
		"openai": &openai.Dialect{}, "anthropic": &anthropic.Dialect{}, "gemini": &gemini.Dialect{},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, responseErr := dialect.ParseResponse([]byte("{"))
			_, streamErr := dialect.ParseStreamChunk([]byte("{"))
			for _, err := range []error{responseErr, streamErr} {
				appErr, ok := apperrors.AsAppError(err)
				if !ok || appErr.Code != apperrors.ErrCodeExternalService || appErr.Retryable || appErr.Cause == nil {
					t.Fatalf("upstream failure misclassified: %v", err)
				}

			}
		})
	}
}

func TestMissingUpstreamResultsAreNotCallerValidation(t *testing.T) {
	t.Parallel()
	for name, dialect := range map[string]llm.Dialect{
		"openai": &openai.Dialect{}, "gemini": &gemini.Dialect{},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := dialect.ParseResponse([]byte("{}"))
			appErr, ok := apperrors.AsAppError(err)
			if !ok || appErr.Code != apperrors.ErrCodeExternalService || appErr.Retryable {
				t.Fatalf("missing upstream result misclassified: %v", err)
			}
		})
	}
}
