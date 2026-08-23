package embedded

import (
	"errors"
	"strings"

	gogit "github.com/go-git/go-git/v5"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
)

func mapPushError(err error, refspecs []string) error {
	message := redactURLCredentials(err.Error())
	switch {
	case isRemoteAuthError(message):
		return giterr.RemoteAuth(message)
	case isPushRejectedError(err, message):
		return giterr.PushRejected(destinationRefs(refspecs), message)
	default:
		return giterr.Network(errors.New(message))
	}
}

func mapRemoteError(err error) error {
	message := redactURLCredentials(err.Error())
	if isRemoteAuthError(message) {
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

func isRemoteAuthError(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "authentication") ||
		strings.Contains(lower, "authorization") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "access denied") ||
		strings.Contains(lower, "401") ||
		strings.Contains(lower, "403")
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

func redactURLCredentials(message string) string {
	var out strings.Builder
	rest := message
	for {
		idx := strings.Index(rest, "://")
		if idx < 0 {
			out.WriteString(rest)
			return out.String()
		}
		afterScheme := idx + len("://")
		out.WriteString(rest[:afterScheme])
		tail := rest[afterScheme:]
		authorityEnd := strings.IndexFunc(tail, func(r rune) bool {
			return r == '/' || r == '?' || r == '#' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
		})
		if authorityEnd < 0 {
			authorityEnd = len(tail)
		}
		authority := tail[:authorityEnd]
		if at := strings.LastIndex(authority, "@"); at >= 0 {
			out.WriteString("***@")
			out.WriteString(authority[at+1:])
		} else {
			out.WriteString(authority)
		}
		rest = tail[authorityEnd:]
	}
}
