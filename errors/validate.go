package errors

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

var reasonPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,62}$`)

// ValidReason reports whether a nonempty machine reason follows the google.rpc reason contract.
func ValidReason(reason string) bool { return reasonPattern.MatchString(reason) }

// Validate checks the shared public vocabulary before encoding it on any transport.
func (e *AppError) Validate() error {
	if !IsKnownCode(e.Code) {
		return fmt.Errorf("invalid application error code %q", e.Code)
	}
	if !utf8.ValidString(e.Message) || !utf8.ValidString(e.TraceID) || (e.Reason != "" && !ValidReason(e.Reason)) {
		return fmt.Errorf("invalid application error text or reason")
	}
	if e.RetryAfter < 0 || (!e.Retryable && e.RetryAfter != 0) {
		return fmt.Errorf("inconsistent application error retry delay")
	}
	for _, violation := range e.Violations {
		if !ValidReason(string(violation.Reason)) || !utf8.ValidString(violation.Field) || !utf8.ValidString(violation.Message) {
			return fmt.Errorf("invalid application error violation")
		}
	}
	return nil
}
