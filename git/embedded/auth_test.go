package embedded_test

import (
	stderrors "errors"
	"path/filepath"
	"testing"

	gitauth "github.com/kbukum/gokit/git/auth"
	"github.com/kbukum/gokit/git/embedded"
	"github.com/kbukum/gokit/git/internal/model"
)

type tokenProviderFunc func() (string, error)

func (f tokenProviderFunc) Token() (string, error) { return f() }

func TestCloneAcceptsHTTPAuthConfigurationsForLocalRemote(t *testing.T) {
	t.Parallel()

	source := initTestRepo(t)
	remote := createRemote(t, source)

	cases := []struct {
		name      string
		transport gitauth.Transport
	}{
		{name: "token value", transport: gitauth.Token{Username: "user", Value: "token"}},
		{name: "token provider", transport: gitauth.Token{Provider: tokenProviderFunc(func() (string, error) { return "provided", nil })}},
		{name: "token pointer", transport: &gitauth.Token{Value: "token"}},
		{name: "nil token pointer", transport: (*gitauth.Token)(nil)},
		{name: "basic auth", transport: gitauth.BasicAuth{Username: "user", Password: "pass"}},
		{name: "basic auth pointer", transport: &gitauth.BasicAuth{Username: "user", Password: "pass"}},
		{name: "nil basic auth pointer", transport: (*gitauth.BasicAuth)(nil)},
		{name: "nil ssh key pointer", transport: (*gitauth.SSHKey)(nil)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cloneDir := filepath.Join(t.TempDir(), "clone")
			repo, err := embedded.Clone(remote, cloneDir, &model.OpenOptions{Transport: tc.transport})
			if err != nil {
				t.Fatalf("Clone() error: %v", err)
			}
			if repo.Root() != cloneDir {
				t.Fatalf("Root() = %q, want %q", repo.Root(), cloneDir)
			}
		})
	}
}

func TestTransportAuthConfigurationErrors(t *testing.T) {
	t.Parallel()

	source := initTestRepo(t)
	remote := createRemote(t, source)

	cases := []struct {
		name      string
		transport gitauth.Transport
	}{
		{name: "token provider error", transport: gitauth.Token{Provider: tokenProviderFunc(func() (string, error) { return "", stderrors.New("token unavailable") })}},
		{name: "ssh key missing path", transport: gitauth.SSHKey{}},
		{name: "credential helper", transport: gitauth.CredentialHelper{Program: "helper"}},
		{name: "credential helper pointer", transport: &gitauth.CredentialHelper{Program: "helper"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cloneDir := filepath.Join(t.TempDir(), "clone")
			if _, err := embedded.Clone(remote, cloneDir, &model.OpenOptions{Transport: tc.transport}); err == nil {
				t.Fatal("Clone() expected transport configuration error")
			}
		})
	}
}
