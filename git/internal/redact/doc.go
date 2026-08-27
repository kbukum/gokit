// Package redact masks sensitive material in git diagnostics so credentials
// never leak into error messages or logs. It is the single owner of URL
// credential redaction shared by both the embedded and CLI git backends.
package redact
