package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseBearer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		header    string
		wantToken string
		wantOK    bool
	}{
		{"Bearer abc123", "abc123", true},
		{"bearer abc123", "abc123", true},
		{"BEARER abc123", "abc123", true},
		{"Bearer  spaced  ", "spaced", true},
		{"Basic abc123", "", false},
		{"Bearer ", "", false},
		{"Bearer", "", false},
		{"", "", false},
		{"abc", "", false},
	}
	for _, c := range cases {
		got, ok := parseBearer(c.header)
		if ok != c.wantOK || got != c.wantToken {
			t.Errorf("parseBearer(%q) = (%q,%v) want (%q,%v)", c.header, got, ok, c.wantToken, c.wantOK)
		}
	}
}

func TestRequireBearerToken(t *testing.T) {
	t.Parallel()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := RequireBearerToken("s3cret", next)

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"valid", "Bearer s3cret", http.StatusTeapot},
		{"case-insensitive scheme", "bearer s3cret", http.StatusTeapot},
		{"wrong token", "Bearer wrong", http.StatusUnauthorized},
		{"missing header", "", http.StatusUnauthorized},
		{"wrong scheme", "Basic s3cret", http.StatusUnauthorized},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
		if c.header != "" {
			req.Header.Set("Authorization", c.header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: status %d want %d", c.name, rec.Code, c.want)
		}
		if c.want == http.StatusUnauthorized && rec.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Errorf("%s: missing WWW-Authenticate challenge", c.name)
		}
	}
}

func TestRequireBearerTokenEmptyPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("empty token must panic to prevent unauthenticated deploy")
		}
	}()
	RequireBearerToken("", http.NotFoundHandler())
}

func FuzzParseBearer(f *testing.F) {
	for _, s := range []string{"Bearer x", "bearer  y ", "Basic z", "Bearer", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, header string) {
		token, ok := parseBearer(header)
		if ok && token == "" {
			t.Fatalf("parseBearer accepted an empty token from %q", header)
		}
	})
}
