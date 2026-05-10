package giterr

import (
	"fmt"

	"github.com/kbukum/gokit/errors"
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
	return errors.InvalidInput("sign", "commit signing is not supported by the go-git backend")
}
func Network(cause error) *errors.AppError  { return errors.ExternalServiceError("git", cause) }
func Internal(cause error) *errors.AppError { return errors.Internal(cause) }

// EmbeddedUnsupported returns an error indicating the embedded (go-git) backend
// does not support this operation or transport type.
func EmbeddedUnsupported(operation string) *errors.AppError {
	return errors.InvalidInput("backend", "operation not supported by the embedded backend: "+operation)
}

func InvalidTransport(kind string) *errors.AppError {
	return errors.InvalidInput("transport", "unsupported transport auth: "+kind)
}
