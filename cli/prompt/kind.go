package prompt

import (
	"fmt"

	"github.com/kbukum/gokit/errors"
)

// nonInteractiveError is returned when a non-interactive prompt has no usable
// default.
func nonInteractiveError(prompt string) *errors.AppError {
	return errors.InvalidInput("prompt", fmt.Sprintf("non-interactive mode requires a default for: %s", prompt))
}

// closedInput is returned when input closes before the prompt is answered.
func closedInput(prompt string) *errors.AppError {
	return errors.InvalidInput("prompt", fmt.Sprintf("input closed before answering: %s", prompt))
}
