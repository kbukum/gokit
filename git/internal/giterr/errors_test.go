package giterr

import (
	stderrors "errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kbukum/gokit/errors"
)

func TestConstructorsReturnTypedAppErrors(t *testing.T) {
	t.Parallel()

	cause := stderrors.New("boom")
	tests := []struct {
		name       string
		err        *errors.AppError
		code       errors.ErrorCode
		status     int
		message    string
		detailKey  string
		detailWant string
		cause      error
	}{
		{name: "RepoNotFound", err: RepoNotFound("/repo"), code: errors.ErrCodeNotFound, status: http.StatusNotFound, message: "repository", detailKey: "resource", detailWant: "repository"},
		{name: "RefNotFound", err: RefNotFound("main"), code: errors.ErrCodeNotFound, status: http.StatusNotFound, message: "ref", detailKey: "id", detailWant: "main"},
		{name: "RemoteNotFound", err: RemoteNotFound("origin"), code: errors.ErrCodeNotFound, status: http.StatusNotFound, message: "remote", detailKey: "id", detailWant: "origin"},
		{name: "ConfigNotFound", err: ConfigNotFound("user.name"), code: errors.ErrCodeNotFound, status: http.StatusNotFound, message: "config", detailKey: "id", detailWant: "user.name"},
		{name: "AmbiguousRef", err: AmbiguousRef("v1"), code: errors.ErrCodeInvalidInput, status: http.StatusUnprocessableEntity, message: "ambiguous ref: v1", detailKey: "field", detailWant: "ref"},
		{name: "Conflict", err: Conflict("file.txt"), code: errors.ErrCodeConflict, status: http.StatusConflict, message: "merge conflict in file.txt"},
		{name: "CheckedOutBranch", err: CheckedOutBranch("main"), code: errors.ErrCodeConflict, status: http.StatusConflict, message: "checked out branch: main"},
		{name: "DetachedHead", err: DetachedHead(), code: errors.ErrCodeInvalidInput, status: http.StatusUnprocessableEntity, message: "detached HEAD", detailKey: "field", detailWant: "HEAD"},
		{name: "AlreadyExists", err: AlreadyExists("branch", "main"), code: errors.ErrCodeConflict, status: http.StatusConflict, message: "branch already exists: main"},
		{name: "InvalidLineRange", err: InvalidLineRange(3, 1), code: errors.ErrCodeInvalidInput, status: http.StatusUnprocessableEntity, message: "start=3 end=1", detailKey: "field", detailWant: "lineRange"},
		{name: "InvalidPath", err: InvalidPath("../x"), code: errors.ErrCodeInvalidInput, status: http.StatusUnprocessableEntity, message: "invalid path: ../x", detailKey: "field", detailWant: "path"},
		{name: "InvalidConfigKey", err: InvalidConfigKey("bad"), code: errors.ErrCodeInvalidInput, status: http.StatusUnprocessableEntity, message: "invalid config key: bad", detailKey: "field", detailWant: "key"},
		{name: "InvalidArg", err: InvalidArg("mode", "bad"), code: errors.ErrCodeInvalidInput, status: http.StatusUnprocessableEntity, message: "bad", detailKey: "field", detailWant: "mode"},
		{name: "SigningNotSupported", err: SigningNotSupported(), code: errors.ErrCodeInvalidInput, status: http.StatusUnprocessableEntity, message: "signing is not supported", detailKey: "field", detailWant: "sign"},
		{name: "Network", err: Network(cause), code: errors.ErrCodeExternalService, status: http.StatusInternalServerError, message: "git service", detailKey: "service", detailWant: "git", cause: cause},
		{name: "Internal", err: Internal(cause), code: errors.ErrCodeInternal, status: http.StatusInternalServerError, message: "unexpected error", cause: cause},
		{name: "EmbeddedUnsupported", err: EmbeddedUnsupported("push"), code: errors.ErrCodeInvalidInput, status: http.StatusUnprocessableEntity, message: "operation not supported", detailKey: "field", detailWant: "backend"},
		{name: "InvalidTransport", err: InvalidTransport("custom"), code: errors.ErrCodeInvalidInput, status: http.StatusUnprocessableEntity, message: "unsupported transport auth: custom", detailKey: "field", detailWant: "transport"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.err.Code != tt.code {
				t.Fatalf("Code = %s, want %s", tt.err.Code, tt.code)
			}
			if tt.err.HTTPStatus != tt.status {
				t.Fatalf("HTTPStatus = %d, want %d", tt.err.HTTPStatus, tt.status)
			}
			if !strings.Contains(tt.err.Message, tt.message) {
				t.Fatalf("Message = %q, want containing %q", tt.err.Message, tt.message)
			}
			if tt.detailKey != "" && tt.err.Details[tt.detailKey] != tt.detailWant {
				t.Fatalf("Details[%q] = %#v, want %q", tt.detailKey, tt.err.Details[tt.detailKey], tt.detailWant)
			}
			if tt.cause != nil && !stderrors.Is(tt.err, tt.cause) {
				t.Fatalf("error does not wrap cause %v: %v", tt.cause, tt.err)
			}
		})
	}
}
