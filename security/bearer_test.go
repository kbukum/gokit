package security_test

import (
	"net/http"
	"strings"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/security"
)

func TestParseBearerHeader(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		header  http.Header
		token   string
		present bool
		invalid bool
	}{
		{name: "nil"},
		{name: "absent", header: http.Header{"Cookie": {"session=not-a-bearer-token"}}},
		{name: "valid", header: http.Header{"Authorization": {security.BearerAuthScheme + " test-token"}}, token: "test-token", present: true},
		{name: "lowercase header and scheme", header: http.Header{"authorization": {"bearer test-token"}}, token: "test-token", present: true},
		{name: "mixed scheme", header: http.Header{"Authorization": {"bEaReR test-token"}}, token: "test-token", present: true},
		{name: "spaces", header: http.Header{"Authorization": {"Bearer   test-token"}}, token: "test-token", present: true},
		{name: "token alphabet", header: http.Header{"Authorization": {"Bearer azAZ09-._~+/=="}}, token: "azAZ09-._~+/==", present: true},
		{name: "maximum token", header: http.Header{"Authorization": {"Bearer " + strings.Repeat("a", 8192)}}, token: strings.Repeat("a", 8192), present: true},
		{name: "empty header", header: http.Header{"Authorization": {""}}, present: true, invalid: true},
		{name: "nil values", header: http.Header{"Authorization": nil}, present: true, invalid: true},
		{name: "zero values", header: http.Header{"Authorization": {}}, present: true, invalid: true},
		{name: "wrong scheme", header: http.Header{"Authorization": {"Basic test-token"}}, present: true, invalid: true},
		{name: "missing scheme", header: http.Header{"Authorization": {"test-token"}}, present: true, invalid: true},
		{name: "empty scheme", header: http.Header{"Authorization": {" test-token"}}, present: true, invalid: true},
		{name: "empty token", header: http.Header{"Authorization": {"Bearer "}}, present: true, invalid: true},
		{name: "no separator", header: http.Header{"Authorization": {"Bearertest-token"}}, present: true, invalid: true},
		{name: "leading whitespace", header: http.Header{"Authorization": {" Bearer test-token"}}, present: true, invalid: true},
		{name: "trailing whitespace", header: http.Header{"Authorization": {"Bearer test-token "}}, present: true, invalid: true},
		{name: "tab separator", header: http.Header{"Authorization": {"Bearer\ttest-token"}}, present: true, invalid: true},
		{name: "whitespace token", header: http.Header{"Authorization": {"Bearer test token"}}, present: true, invalid: true},
		{name: "padding only", header: http.Header{"Authorization": {"Bearer =="}}, present: true, invalid: true},
		{name: "interior padding", header: http.Header{"Authorization": {"Bearer a=b"}}, present: true, invalid: true},
		{name: "Unicode", header: http.Header{"Authorization": {"Bearer résumé"}}, present: true, invalid: true},
		{name: "control", header: http.Header{"Authorization": {"Bearer test\x00token"}}, present: true, invalid: true},
		{name: "newline", header: http.Header{"Authorization": {"Bearer test\r\ntoken"}}, present: true, invalid: true},
		{name: "comma joined", header: http.Header{"Authorization": {"Bearer test-token,Bearer other"}}, present: true, invalid: true},
		{name: "duplicate", header: http.Header{"Authorization": {"Bearer test-token", "Bearer other"}}, present: true, invalid: true},
		{name: "duplicate empty", header: http.Header{"Authorization": {"", "Bearer other"}}, present: true, invalid: true},
		{name: "canonical alias duplicate", header: http.Header{"Authorization": {"Bearer test-token"}, "authorization": {"Bearer other"}}, present: true, invalid: true},
		{name: "empty alias duplicate", header: http.Header{"Authorization": {"Bearer test-token"}, "AUTHORIZATION": nil}, present: true, invalid: true},
		{name: "oversized token", header: http.Header{"Authorization": {"Bearer " + strings.Repeat("a", 8193)}}, present: true, invalid: true},
		{name: "oversized whitespace", header: http.Header{"Authorization": {"Bearer " + strings.Repeat(" ", 8192) + "a"}}, present: true, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			token, present, err := security.ParseBearerHeader(tc.header)
			if token != tc.token || present != tc.present || (err != nil) != tc.invalid {
				t.Fatalf("token length=%d, present=%v, error=%v; want length=%d, present=%v, invalid=%v", len(token), present, err, len(tc.token), tc.present, tc.invalid)
			}
			if err != nil && apperrors.Normalize(err).Code != apperrors.ErrCodeUnauthorized {
				t.Fatalf("invalid credential error category: %v", err)
			}
		})
	}
}

func FuzzParseBearerHeader(f *testing.F) {
	for _, value := range []string{"", "Bearer test-token", "Bearer ==", "Bearer a=b", "Bearer a==", "Bearer a,b", "Bearer\ta", "Bearer " + strings.Repeat("a", 8193)} {
		f.Add(value, false)
		f.Add(value, true)
	}
	f.Fuzz(func(t *testing.T, value string, duplicate bool) {
		header := http.Header{"Authorization": {value}}
		if duplicate {
			header["authorization"] = []string{value}
		}
		token, present, err := security.ParseBearerHeader(header)
		if !present {
			t.Fatal("present header became missing")
		}
		if err != nil {
			if token != "" {
				t.Fatal("invalid credential returned a usable token")
			}
			return
		}
		if duplicate || token == "" || len(token) > 8192 || strings.ContainsAny(token, " \t\r\n,") {
			t.Fatal("malformed credential accepted")
		}
	})
}
