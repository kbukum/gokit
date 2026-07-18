package prompt

import (
	"fmt"
	"strings"

	"github.com/kbukum/gokit/errors"
)

// runText asks for freeform text with an optional default and optional validation.
//
// In non-interactive mode it resolves to the default (a typed error when absent or rejected by the validator). Interactively it prints the prompt and reads a line; a blank answer accepts the default, and a validator rejection is shown before re-asking. hasDefault distinguishes "" (a real empty default) from no default at all.
func runText(term Terminal, style Style, mode PromptMode, prompt, def string, hasDefault bool, validator Validator) (string, error) {
	if !mode.IsInteractive() {
		if !hasDefault {
			return "", nonInteractiveError(prompt)
		}
		if validator != nil {
			if err := validator.Validate(def); err != nil {
				return "", errors.InvalidInput("prompt", fmt.Sprintf("default for %s is invalid: %s", prompt, err))
			}
		}
		return def, nil
	}

	if err := term.WriteLine(heading(style, prompt)); err != nil {
		return "", err
	}
	for {
		hint := ""
		if hasDefault && def != "" {
			hint = "[" + def + "]"
		}
		if err := writeAnswer(term, style, hint); err != nil {
			return "", err
		}
		line, ok, err := term.ReadLine()
		if err != nil {
			return "", err
		}
		if !ok {
			return "", closedInput(prompt)
		}
		value, accepted, reason := acceptText(strings.TrimSpace(line), def, hasDefault, validator)
		if accepted {
			return value, nil
		}
		if err := notice(term, style, reason); err != nil {
			return "", err
		}
	}
}

// acceptText resolves a raw answer against the default and validator, returning the accepted value or a rejection reason to display before re-asking.
func acceptText(value, def string, hasDefault bool, validator Validator) (resolved string, accepted bool, reason string) {
	resolved = value
	if value == "" {
		if !hasDefault {
			return "", false, "a value is required"
		}
		resolved = def
	}
	if validator != nil {
		if err := validator.Validate(resolved); err != nil {
			return "", false, err.Error()
		}
	}
	return resolved, true, ""
}
