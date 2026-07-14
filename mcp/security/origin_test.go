package security

import "testing"

func TestValidateAllowedOrigin(t *testing.T) {
	t.Parallel()
	valid := []struct{ in, want string }{
		{"https://app.example.com", "https://app.example.com"},
		{"HTTP://App.Example.COM:8080", "http://app.example.com:8080"},
		{"https://example.com/", "https://example.com"},
	}
	for _, c := range valid {
		got, err := ValidateAllowedOrigin(c.in)
		if err != nil {
			t.Errorf("%q: unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("%q: got %q want %q", c.in, got, c.want)
		}
	}
	invalid := []string{
		"ftp://example.com",
		"https://example.com/path",
		"https://example.com?q=1",
		"https://example.com#frag",
		"******example.com",
		"https://",
		"mailto:foo@example.com",
		"://noscheme",
		"not a url at all\x7f",
	}
	for _, in := range invalid {
		if _, err := ValidateAllowedOrigin(in); err == nil {
			t.Errorf("%q: expected rejection", in)
		}
	}
}

func FuzzValidateAllowedOrigin(f *testing.F) {
	for _, s := range []string{"https://a.com", "http://a.com:1/", "ftp://x", "://y", "https://u@h", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, origin string) {
		got, err := ValidateAllowedOrigin(origin)
		if err != nil {
			return
		}
		// A normalized, accepted origin must re-validate to itself (idempotent).
		again, err := ValidateAllowedOrigin(got)
		if err != nil {
			t.Fatalf("normalized origin %q re-rejected: %v", got, err)
		}
		if again != got {
			t.Fatalf("normalization not idempotent: %q -> %q", got, again)
		}
	})
}
