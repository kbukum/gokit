package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kbukum/gokit/util"
)

func TestFileSecretsValidateBoundedContents(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, content, want string
		invalid             bool
	}{
		{name: "plain", content: "synthetic", want: "synthetic"},
		{name: "newline", content: "synthetic\n", want: "synthetic"},
		{name: "crlf", content: "synthetic\r\n", want: "synthetic"},
		{name: "preserve spaces", content: " synthetic ", want: " synthetic "},
		{name: "empty", invalid: true},
		{name: "newline only", content: "\n", invalid: true},
		{name: "nul", content: "synthetic\x00", invalid: true},
		{name: "too large", content: strings.Repeat("x", 4097), invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "credential")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			value, err := (FileSecrets{}).ReadSecret(t.Context(), path)
			if tc.invalid {
				if err == nil || !value.IsEmpty() {
					t.Fatal("invalid secret returned a usable value")
				}
			} else if err != nil || value.Expose() != tc.want {
				t.Fatal("secret read changed contents or failed")
			}
		})
	}
}

type injectedSecrets struct {
	calls int
}

func (s *injectedSecrets) ReadSecret(context.Context, string) (util.SecretString, error) {
	s.calls++
	return util.NewSecretString("synthetic"), nil
}

func TestResolvePasswordUsesOnlyExplicitSource(t *testing.T) {
	t.Parallel()
	source := &injectedSecrets{}
	value, err := ResolvePassword(t.Context(), ConnParams{PasswordFile: "injected"}, source)
	if err != nil || source.calls != 1 || value.Expose() != "synthetic" {
		t.Fatal("injected source was not used")
	}

	if _, err := ResolvePassword(t.Context(), ConnParams{Password: "synthetic", PasswordFile: "injected"}, source); err == nil || source.calls != 1 {
		t.Fatal("ambiguous password configuration read a secret")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ResolvePassword(ctx, ConnParams{PasswordFile: "injected"}, source); !errors.Is(err, context.Canceled) || source.calls != 1 {
		t.Fatal("canceled preparation read a secret")
	}
	var nilSource *injectedSecrets
	if _, err := ResolvePassword(t.Context(), ConnParams{PasswordFile: "injected"}, nilSource); err == nil {
		t.Fatal("typed-nil secret source accepted")
	}
}
