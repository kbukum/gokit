package validation

import (
	"strings"

	"github.com/kbukum/gokit/errors"
)

// ReasonForRule classifies validator constraints into stable semantics. Unrecognized or custom constraints remain INVALID_VALUE; raw IDs never become wire identities.
func ReasonForRule(rule string) errors.ViolationReason {
	_, tail, found := strings.Cut(rule, ".")
	if found {
		rule = tail
	}
	switch rule {
	case "required":
		return errors.ViolationRequired
	case "email", "url", "uri", "uuid", "hostname", "ip", "ipv4", "ipv6", "pattern":
		return errors.ViolationInvalidFormat
	case "min", "max", "min_len", "max_len", "min_bytes", "max_bytes", "min_items", "max_items", "gte", "lte", "gt", "lt", "range", "before", "after":
		return errors.ViolationOutOfRange
	default:
		return errors.ViolationInvalidValue
	}
}
