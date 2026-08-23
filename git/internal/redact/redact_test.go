package redact_test

import (
	"testing"

	"github.com/kbukum/gokit/git/internal/redact"
)

func TestURLCredentials(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no url", "fatal: repository not found", "fatal: repository not found"},
		{"no userinfo", "clone https://example.com/repo.git", "clone https://example.com/repo.git"},
		{"user and password", "https://alice:s3cret@example.com/repo.git", "https://alice:***@example.com/repo.git"},
		{"bare token in username", "https://ghp_token@example.com/repo.git", "https://***@example.com/repo.git"},
		{"ssh scheme", "ssh://git:key@example.com:22/repo.git", "ssh://git:***@example.com:22/repo.git"},
		{"port not leaked as scheme", "dial tcp example.com:4030: refused", "dial tcp example.com:4030: refused"},
		{"multiple urls", "a https://u:p@h1/x b https://tok@h2/y", "a https://u:***@h1/x b https://***@h2/y"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := redact.URLCredentials(tc.in); got != tc.want {
				t.Fatalf("URLCredentials(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
