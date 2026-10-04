package security

import (
	"net/http"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
)

// MaxBearerTokenBytes bounds the credential passed to a bearer validator.
const MaxBearerTokenBytes = 8192

// ParseBearerHeader accepts one Authorization header containing an RFC 6750 bearer credential. Header names and the scheme are case-insensitive; token bytes are not. Tokens are bounded to MaxBearerTokenBytes and the complete value to that limit plus the canonical scheme separator. Only an absent header returns present=false. Empty values, repeated fields (including differently cased map keys), invalid syntax, and oversized values return an unauthorized error without credential diagnostics.
func ParseBearerHeader(header http.Header) (token string, present bool, err error) {
	var value string
	for name, values := range header {
		if !strings.EqualFold(name, "Authorization") {
			continue
		}
		if present || len(values) != 1 {
			return "", true, apperrors.Unauthorized("")
		}
		present = true
		value = values[0]
	}
	if !present {
		return "", false, nil
	}
	if len(value) > len(BearerAuthScheme)+1+MaxBearerTokenBytes {
		return "", true, apperrors.Unauthorized("")
	}
	scheme, credential, separated := strings.Cut(value, " ")
	if !separated || !strings.EqualFold(scheme, BearerAuthScheme) {
		return "", true, apperrors.Unauthorized("")
	}
	credential = strings.TrimLeft(credential, " ")
	if !validBearerCredential(credential) {
		return "", true, apperrors.Unauthorized("")
	}
	return credential, true, nil
}

func validBearerCredential(token string) bool {
	if token == "" {
		return false
	}
	padding := false
	for i := range len(token) {
		c := token[i]
		if c == '=' {
			if i == 0 {
				return false
			}
			padding = true
			continue
		}
		allowed := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~+/", rune(c))
		if padding || !allowed {
			return false
		}
	}
	return true
}
