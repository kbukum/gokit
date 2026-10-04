package testutil_test

import (
	"errors"
	"testing"

	"github.com/kbukum/gokit/sse/testutil"
)

func TestTokenValidatorTypedClaimsAndCause(t *testing.T) {
	t.Parallel()
	type principal struct{ Subject string }
	want := principal{Subject: "alice"}
	validator := testutil.TokenValidator[principal]{Claims: want}
	got, err := validator.ValidateToken(t.Context(), "test-token")
	if err != nil || got != want {
		t.Fatalf("typed claims: %+v, %v", got, err)
	}
	cause := errors.New("private rejection")
	validator.Err = cause
	if _, err := validator.ValidateToken(t.Context(), "test-token"); !errors.Is(err, cause) {
		t.Fatalf("diagnostic lost: %v", err)
	}
}
