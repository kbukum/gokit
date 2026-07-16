package prompt

import "strings"

// Validator inspects a candidate answer, accepting it or explaining why it is
// rejected.
//
// In an interactive prompt a rejection reason is shown and the question is
// re-asked; in [ModeNonInteractive] a rejected default is a typed error rather
// than a silent bad value.
type Validator interface {
	// Validate returns nil to accept input or an error whose message is the
	// human-readable rejection reason.
	Validate(input string) error
}

// ValidatorFunc adapts a plain function into a [Validator].
type ValidatorFunc func(input string) error

// Validate calls the underlying function.
func (f ValidatorFunc) Validate(input string) error { return f(input) }

// NonEmpty returns a validator that rejects empty or whitespace-only input with
// message.
func NonEmpty(message string) Validator {
	return ValidatorFunc(func(input string) error {
		if strings.TrimSpace(input) == "" {
			return rejection(message)
		}
		return nil
	})
}

// rejection is a lightweight error carrying only a human-readable reason, used
// as a validator's rejection signal.
type rejection string

func (r rejection) Error() string { return string(r) }
