package password

import (
	"strings"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestBcryptMalformedEncodingIsNotMismatch(t *testing.T) {
	t.Parallel()
	h := newHasher(t, Config{Algorithm: AlgorithmBcrypt, MinLength: 4})
	for _, hash := range []string{
		"$2a$12$" + strings.Repeat("!", 53),
		"$2z$12$" + strings.Repeat("a", 53),
		"$2a$12$" + strings.Repeat("a", 52),
		"$2a$12$" + strings.Repeat("a", 54),
		"$2a$12$" + strings.Repeat("a", 22) + strings.Repeat("!", 31),
	} {
		requireError(t, h.Verify("password", hash), apperrors.ErrCodeInternal, ReasonCorruptHash)
	}
}
