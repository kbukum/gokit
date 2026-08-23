package embedded

import (
	"errors"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/redact"
)

func mapPushError(err error, refspecs []string) error {
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
	message := redact.URLCredentials(err.Error())
	if isRemoteAuthError(err, message) {
		return giterr.RemoteAuth(message)
	}
	return giterr.Network(errors.New(message))
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
