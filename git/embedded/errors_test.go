package embedded

import (
	stderrors "errors"
	"strings"
	"testing"

	goerrors "github.com/kbukum/gokit/errors"
)

func TestMapPushErrorClassifiesRejectedRefAndRedactsCredentials(t *testing.T) {
	t.Parallel()

	err := mapPushError(stderrors.New("https://token:secret@example.com/repo.git rejected: non-fast-forward"), []string{"refs/heads/main:refs/heads/main"})

	var appErr *goerrors.AppError
	if !stderrors.As(err, &appErr) {
		t.Fatalf("error = %T %v, want AppError", err, err)
	}
	if appErr.Details["gokit_git_error"] != "push_rejected" {
		t.Fatalf("gokit_git_error = %#v, want push_rejected", appErr.Details["gokit_git_error"])
	}
	if appErr.Details["refname"] != "refs/heads/main" {
		t.Fatalf("refname = %#v, want refs/heads/main", appErr.Details["refname"])
	}
	if strings.Contains(appErr.Message, "secret") {
		t.Fatalf("push rejection leaked credential: %q", appErr.Message)
	}
}

func TestMapPushErrorClassifiesRemoteAuth(t *testing.T) {
	t.Parallel()

	err := mapPushError(stderrors.New("https://user:pass@example.com/repo.git authentication required"), nil)

	var appErr *goerrors.AppError
	if !stderrors.As(err, &appErr) {
		t.Fatalf("error = %T %v, want AppError", err, err)
	}
	if appErr.Details["gokit_git_error"] != "remote_auth" {
		t.Fatalf("gokit_git_error = %#v, want remote_auth", appErr.Details["gokit_git_error"])
	}
	if strings.Contains(appErr.Message, "pass") {
		t.Fatalf("remote auth error leaked credential: %q", appErr.Message)
	}
}
