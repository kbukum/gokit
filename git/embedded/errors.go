package embedded

import (
	"context"
	"errors"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/redact"
)

func mapPushError(err error, refspecs []string) error {
	if ctxErr := contextCause(err); ctxErr != nil {
		return giterr.Network(ctxErr)
	}
	message := redact.URLCredentials(err.Error())
	switch {
	case isRemoteAuthError(err, message):
		return giterr.RemoteAuth(message)
	case isPushRejectedError(err, message):
		return giterr.PushRejected(destinationRefs(refspecs), message)
	default:
		return giterr.Network(errors.New(message))
	}
}

func mapRemoteError(err error) error {
	if ctxErr := contextCause(err); ctxErr != nil {
		return giterr.Network(ctxErr)
	}
	message := redact.URLCredentials(err.Error())
	if isRemoteAuthError(err, message) {
		return giterr.RemoteAuth(message)
	}
	return giterr.Network(errors.New(message))
}

// contextCause returns the context sentinel (Canceled or DeadlineExceeded) when
// err was produced by a canceled or timed-out remote call, and nil otherwise.
// The sentinel is preserved as the error cause so callers can detect cancellation
// with errors.Is; context sentinels carry no credentials, so no redaction is lost.
func contextCause(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func isPushRejectedError(err error, message string) bool {
	lower := strings.ToLower(message)
	return errors.Is(err, gogit.ErrNonFastForwardUpdate) ||
		strings.Contains(lower, "non-fast-forward") ||
		strings.Contains(lower, "non fast forward") ||
		strings.Contains(lower, "rejected") ||
		strings.Contains(lower, "protected branch") ||
		strings.Contains(lower, "pre-receive hook declined")
}

func isRemoteAuthError(err error, message string) bool {
	if errors.Is(err, transport.ErrAuthenticationRequired) || errors.Is(err, transport.ErrAuthorizationFailed) {
		return true
	}
	lower := strings.ToLower(message)
	return strings.Contains(lower, "authentication") ||
		strings.Contains(lower, "authorization") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "access denied") ||
		strings.Contains(lower, "401 unauthorized") ||
		strings.Contains(lower, "403 forbidden")
}

func destinationRefs(refspecs []string) string {
	if len(refspecs) == 0 {
		return "the remote"
	}
	refs := make([]string, 0, len(refspecs))
	for _, refspec := range refspecs {
		refspec = strings.TrimPrefix(refspec, "+")
		if _, dst, ok := strings.Cut(refspec, ":"); ok {
			refs = append(refs, dst)
		} else {
			refs = append(refs, refspec)
		}
	}
	return strings.Join(refs, ", ")
}
