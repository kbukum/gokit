package git

import (
	"github.com/kbukum/gokit/errors"
	giterr "github.com/kbukum/gokit/git/internal/giterr"
)

func ErrRepoNotFound(path string) *errors.AppError        { return giterr.RepoNotFound(path) }
func ErrRefNotFound(refname string) *errors.AppError      { return giterr.RefNotFound(refname) }
func ErrRemoteNotFound(name string) *errors.AppError      { return giterr.RemoteNotFound(name) }
func ErrConfigNotFound(key string) *errors.AppError       { return giterr.ConfigNotFound(key) }
func ErrAmbiguousRef(refname string) *errors.AppError     { return giterr.AmbiguousRef(refname) }
func ErrConflict(path string) *errors.AppError            { return giterr.Conflict(path) }
func ErrCheckedOutBranch(name string) *errors.AppError    { return giterr.CheckedOutBranch(name) }
func ErrDetachedHead() *errors.AppError                   { return giterr.DetachedHead() }
func ErrAlreadyExists(kind, name string) *errors.AppError { return giterr.AlreadyExists(kind, name) }
func ErrInvalidLineRange(start, end int) *errors.AppError { return giterr.InvalidLineRange(start, end) }
func ErrInvalidPath(path string) *errors.AppError         { return giterr.InvalidPath(path) }
func ErrInvalidConfigKey(key string) *errors.AppError     { return giterr.InvalidConfigKey(key) }
func ErrSigningNotSupported() *errors.AppError            { return giterr.SigningNotSupported() }
func ErrNetwork(cause error) *errors.AppError             { return giterr.Network(cause) }
func ErrInternal(cause error) *errors.AppError            { return giterr.Internal(cause) }
func ErrEmbeddedUnsupported(operation string) *errors.AppError {
	return giterr.EmbeddedUnsupported(operation)
}
func ErrInvalidTransport(kind string) *errors.AppError { return giterr.InvalidTransport(kind) }
