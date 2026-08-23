// Package redact masks sensitive material in git diagnostics so credentials
// never leak into error messages or logs. It is the single owner of URL
// credential redaction shared by both the embedded and CLI backends.
package redact

import "strings"

// URLCredentials masks credential material in the userinfo component of every
// "scheme://userinfo@host" occurrence in message, for any URL scheme. When the
// userinfo carries a "user:password" pair, the (non-secret) username is kept and
// the password is masked ("user:***@host"). A bare userinfo with no password
// ("token@host") is masked in full ("***@host"), because git commonly places the
// secret in the username position (e.g. "https://<token>@host").
func URLCredentials(message string) string {
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
			userinfo := authority[:at]
			host := authority[at+1:]
			if user, _, ok := strings.Cut(userinfo, ":"); ok {
				out.WriteString(user)
				out.WriteString(":***@")
			} else {
				out.WriteString("***@")
			}
			out.WriteString(host)
		} else {
			out.WriteString(authority)
		}
		rest = tail[authorityEnd:]
	}
}
