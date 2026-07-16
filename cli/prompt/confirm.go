package prompt

import "strings"

// runConfirm asks a yes/no question with an explicit default.
//
// In non-interactive mode it resolves to default without touching the terminal;
// otherwise it prints the prompt and parses y/yes/n/no (blank accepts the
// default), re-asking on unrecognized input.
func runConfirm(term Terminal, style Style, mode PromptMode, prompt string, def bool) (bool, error) {
	if !mode.IsInteractive() {
		return def, nil
	}
	for {
		if err := term.Write(heading(style, prompt) + " " + confirmSuffix(def) + ": "); err != nil {
			return false, err
		}
		if err := term.Flush(); err != nil {
			return false, err
		}
		line, ok, err := term.ReadLine()
		if err != nil {
			return false, err
		}
		if !ok {
			return false, closedInput(prompt)
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			if err := notice(term, style, "please answer 'y' or 'n'"); err != nil {
				return false, err
			}
		}
	}
}

func confirmSuffix(def bool) string {
	if def {
		return "[Y/n]"
	}
	return "[y/N]"
}
