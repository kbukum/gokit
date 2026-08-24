package git_test

import (
	"errors"
	"testing"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/git"
)

// TestPublicErrorConstructorsMapToStableCodes pins the public typed-error surface:
// each git error constructor must yield a non-nil *AppError with a stable code so
// callers can branch on it. A regression in the internal mapping fails here.
func TestPublicErrorConstructorsMapToStableCodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  *goerrors.AppError
		want goerrors.ErrorCode
	}{
		{"RepoNotFound", git.ErrRepoNotFound("/repo"), goerrors.ErrCodeNotFound},
		{"RefNotFound", git.ErrRefNotFound("main"), goerrors.ErrCodeNotFound},
		{"RemoteNotFound", git.ErrRemoteNotFound("origin"), goerrors.ErrCodeNotFound},
		{"ConfigNotFound", git.ErrConfigNotFound("user.name"), goerrors.ErrCodeNotFound},
		{"AmbiguousRef", git.ErrAmbiguousRef("v1"), goerrors.ErrCodeInvalidInput},
		{"Conflict", git.ErrConflict("file.txt"), goerrors.ErrCodeConflict},
		{"CheckedOutBranch", git.ErrCheckedOutBranch("main"), goerrors.ErrCodeConflict},
		{"DetachedHead", git.ErrDetachedHead(), goerrors.ErrCodeInvalidInput},
		{"AlreadyExists", git.ErrAlreadyExists("branch", "main"), goerrors.ErrCodeConflict},
		{"InvalidLineRange", git.ErrInvalidLineRange(5, 1), goerrors.ErrCodeInvalidInput},
		{"InvalidPath", git.ErrInvalidPath("../etc"), goerrors.ErrCodeInvalidInput},
		{"InvalidConfigKey", git.ErrInvalidConfigKey("bad"), goerrors.ErrCodeInvalidInput},
		{"SigningNotSupported", git.ErrSigningNotSupported(), goerrors.ErrCodeInvalidInput},
		{"SigningKeyMissing", git.ErrSigningKeyMissing("user.signingkey"), goerrors.ErrCodeInvalidInput},
		{"IdentityMissing", git.ErrIdentityMissing("user.email"), goerrors.ErrCodeInvalidInput},
		{"FileTooLarge", git.ErrFileTooLarge("f", "HEAD", 10, 5), goerrors.ErrCodeInvalidInput},
		{"RemoteAuth", git.ErrRemoteAuth("denied"), goerrors.ErrCodeUnauthorized},
		{"PushRejected", git.ErrPushRejected("main", "non-fast-forward"), goerrors.ErrCodeConflict},
		{"Network", git.ErrNetwork(errors.New("boom")), goerrors.ErrCodeExternalService},
		{"Internal", git.ErrInternal(errors.New("boom")), goerrors.ErrCodeInternal},
		{"EmbeddedUnsupported", git.ErrEmbeddedUnsupported("op"), goerrors.ErrCodeInvalidInput},
		{"InvalidTransport", git.ErrInvalidTransport("kind"), goerrors.ErrCodeInvalidInput},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.err == nil {
				t.Fatal("constructor returned nil")
			}
			if tc.err.Code != tc.want {
				t.Fatalf("%s code = %q, want %q", tc.name, tc.err.Code, tc.want)
			}
		})
	}
}

// TestNetworkErrorPreservesCause proves the cause chain is preserved so callers
// can inspect the underlying error with errors.Is/As.
func TestNetworkErrorPreservesCause(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("underlying transport failure")
	err := git.ErrNetwork(sentinel)
	if !errors.Is(err, sentinel) {
		t.Fatalf("ErrNetwork does not preserve cause: %v", err)
	}
}
