package prompt

import (
	"fmt"
	"strings"

	"github.com/kbukum/gokit/errors"
)

// recommendedIndices returns the indices of every recommended choice, in order.
func recommendedIndices(choices []Choice) []int {
	var indices []int
	for i, c := range choices {
		if c.IsRecommended() {
			indices = append(indices, i)
		}
	}
	return indices
}

// ids maps choice indices to their [ChoiceID]s.
func ids(choices []Choice, indices []int) []ChoiceID {
	out := make([]ChoiceID, len(indices))
	for i, index := range indices {
		out[i] = choices[index].ID()
	}
	return out
}

// runMultiSelect asks for zero or more choices.
//
// In non-interactive mode it resolves to the set of recommended choices (which
// may be empty). Interactively it prints a numbered list and reads a
// comma-separated list of one-based numbers; a blank answer accepts the
// recommended defaults.
func runMultiSelect(term Terminal, style Style, mode PromptMode, prompt string, choices []Choice) ([]ChoiceID, error) {
	if len(choices) == 0 {
		return nil, errors.InvalidInput("prompt", fmt.Sprintf("multi-select requires at least one choice: %s", prompt))
	}
	defaults := recommendedIndices(choices)

	if !mode.IsInteractive() {
		return ids(choices, defaults), nil
	}

	if err := term.WriteLine(heading(style, prompt)); err != nil {
		return nil, err
	}
	for _, row := range numberedRows(style, choices) {
		if err := term.WriteLine(row); err != nil {
			return nil, err
		}
	}
	for {
		if err := writeAnswer(term, style, multiHint(defaults)); err != nil {
			return nil, err
		}
		line, ok, err := term.ReadLine()
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, closedInput(prompt)
		}
		answer := strings.TrimSpace(line)
		if answer == "" {
			return ids(choices, defaults), nil
		}
		if indices, valid := parseIndices(answer, len(choices)); valid {
			return ids(choices, indices), nil
		}
		if err := notice(term, style, fmt.Sprintf("enter comma-separated numbers between 1 and %d", len(choices))); err != nil {
			return nil, err
		}
	}
}

// multiHint renders the default-selection hint (e.g. "[1,3]" or "[none]").
func multiHint(defaults []int) string {
	if len(defaults) == 0 {
		return "[none]"
	}
	parts := make([]string, len(defaults))
	for i, index := range defaults {
		parts[i] = fmt.Sprintf("%d", index+1)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// parseIndices parses a comma-separated list of one-based numbers into distinct
// zero-based indices; the second return value is false when any token is out of
// range or unparsable.
func parseIndices(input string, length int) ([]int, bool) {
	var indices []int
	seen := make(map[int]bool)
	for _, token := range strings.Split(input, ",") {
		index, valid := parseIndex(token, length)
		if !valid {
			return nil, false
		}
		if !seen[index] {
			seen[index] = true
			indices = append(indices, index)
		}
	}
	return indices, true
}
