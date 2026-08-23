package giterr

import (
	"fmt"

	"github.com/kbukum/gokit/errors"
)

const (
	DetailGitError = "gokit_git_error"

	DetailFileTooLarge       = "file_too_large"
	DetailSigningKeyMissing  = "signing_key_missing"
	DetailIdentityMissing    = "identity_missing"
	DetailRemoteAuth         = "remote_auth"
	DetailPushRejected       = "push_rejected"
	signingKeyMissingHint    = "Configure a signing key with git config user.signingkey <key>, then retry the signed tag."
	identityMissingHint      = "Set it with git config user.name/user.email"
	pushRejectedHint         = "Integrate remote changes, or land via PR and push tags only."
	remoteAuthenticationHint = "Check remote credentials and token/key push permissions."
)

func RepoNotFound(path string) *errors.AppError   { return errors.NotFound("repository", path) }
func RefNotFound(refname string) *errors.AppError { return errors.NotFound("ref", refname) }
func RemoteNotFound(name string) *errors.AppError { return errors.NotFound("remote", name) }
func ConfigNotFound(key string) *errors.AppError  { return errors.NotFound("config", key) }
func AmbiguousRef(refname string) *errors.AppError {
	return errors.InvalidInput("ref", "ambiguous ref: "+refname)
}
func Conflict(path string) *errors.AppError { return errors.Conflict("merge conflict in " + path) }
func CheckedOutBranch(name string) *errors.AppError {
	return errors.Conflict("cannot delete checked out branch: " + name)
}
func DetachedHead() *errors.AppError { return errors.InvalidInput("HEAD", "detached HEAD") }
func AlreadyExists(kind, name string) *errors.AppError {
	return errors.Conflict(fmt.Sprintf("%s already exists: %s", kind, name))
}

func InvalidLineRange(start, end int) *errors.AppError {
	return errors.InvalidInput("lineRange", fmt.Sprintf("invalid line range: start=%d end=%d", start, end))
}

func InvalidPath(path string) *errors.AppError {
	return errors.InvalidInput("path", "invalid path: "+path)
}

func InvalidConfigKey(key string) *errors.AppError {
	return errors.InvalidInput("key", "invalid config key: "+key)
}

func InvalidArg(field, detail string) *errors.AppError {
	return errors.InvalidInput(field, detail)
}

func SigningNotSupported() *errors.AppError {
	return errors.InvalidInput("sign", "signing is not supported by the go-git backend")
}

func SigningKeyMissing(key string) *errors.AppError {
	return errors.InvalidInput("sign", key+" is not configured").WithDetails(map[string]any{
		DetailGitError: DetailSigningKeyMissing,
		"key":          key,
		"hint":         signingKeyMissingHint,
	})
}

func IdentityMissing(key string) *errors.AppError {
	return errors.InvalidInput("git identity", key+" is not configured").WithDetails(map[string]any{
		DetailGitError: DetailIdentityMissing,
		"key":          key,
		"hint":         identityMissingHint,
	})
}

func FileTooLarge(path, revision string, size, limit int64) *errors.AppError {
	return errors.InvalidInput("path", fmt.Sprintf("file %q at %q is %d bytes, exceeding limit %d bytes", path, revision, size, limit)).WithDetails(map[string]any{
		DetailGitError: DetailFileTooLarge,
		"path":         path,
		"revision":     revision,
		"size":         size,
		"max":          limit,
	})
}

func RemoteAuth(message string) *errors.AppError {
	return errors.Unauthorized("git remote authentication failed: " + message).WithDetails(map[string]any{
		DetailGitError: DetailRemoteAuth,
		"hint":         remoteAuthenticationHint,
	})
}

func PushRejected(refname, reason string) *errors.AppError {
	return errors.Conflict("remote rejected push to " + refname + ": " + reason).WithDetails(map[string]any{
		DetailGitError: DetailPushRejected,
		"refname":      refname,
		"hint":         pushRejectedHint,
	})
}
func Network(cause error) *errors.AppError  { return errors.ExternalServiceError("git", cause) }
func Internal(cause error) *errors.AppError { return errors.Internal(cause) }

// EmbeddedUnsupported returns an error indicating the embedded (go-git) backend does not support this operation
// or transport type.
func EmbeddedUnsupported(operation string) *errors.AppError {
	return errors.InvalidInput("backend", "operation not supported by the embedded backend: "+operation)
}

func InvalidTransport(kind string) *errors.AppError {
	return errors.InvalidInput("transport", "unsupported transport auth: "+kind)
}
