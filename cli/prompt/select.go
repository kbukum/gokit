package prompt

import (
	"fmt"
	"strings"

	"github.com/kbukum/gokit/errors"
)

// recommendedIndex returns the index of the first recommended choice, or -1.
func recommendedIndex(choices []Choice) int {
	for i, c := range choices {
		if c.IsRecommended() {
			return i
		}
	}
	return -1
}

// runSelect asks for exactly one choice.
//
// In non-interactive mode it resolves to the recommended choice (a typed error when none is marked).
// Interactively it prints a numbered list and reads a one-based number;
// a blank answer accepts the default when present.
func (s session) runSelect(prompt string, choices []Choice) (ChoiceID, error) {
	if len(choices) == 0 {
		return "", errors.InvalidInput("prompt", fmt.Sprintf("select requires at least one choice: %s", prompt))
	}
	def := recommendedIndex(choices)

	if !s.mode.IsInteractive() {
		if def < 0 {
			return "", nonInteractiveError(prompt)
		}
		return choices[def].ID(), nil
	}

	if err := s.term.WriteLine(heading(s.style, prompt)); err != nil {
		return "", err
	}
	for _, row := range numberedRows(s.style, choices) {
		if err := s.term.WriteLine(row); err != nil {
			return "", err
		}
	}
	for {
		hint := ""
		if def >= 0 {
			hint = fmt.Sprintf("[%d]", def+1)
		}
		if err := writeAnswer(s.term, s.style, hint); err != nil {
			return "", err
		}
		line, ok, err := s.term.ReadLine()
		if err != nil {
			return "", err
		}
		if !ok {
			return "", closedInput(prompt)
		}
		answer := strings.TrimSpace(line)
		if answer == "" {
			if def >= 0 {
				return choices[def].ID(), nil
			}
			if err := notice(s.term, s.style, "a choice is required"); err != nil {
				return "", err
			}
			continue
		}
		if index, valid := parseIndex(answer, len(choices)); valid {
			return choices[index].ID(), nil
		}
		if err := notice(s.term, s.style, fmt.Sprintf("enter a number between 1 and %d", len(choices))); err != nil {
			return "", err
		}
	}
}
