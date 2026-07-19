package prompt

import "strings"

// runConfirm asks a yes/no question with an explicit default.
//
// In non-interactive mode it resolves to default without touching the terminal;
// otherwise it prints the prompt and parses y/yes/n/no (blank accepts the default),
// re-asking on unrecognized input.
func (s session) runConfirm(prompt string, def bool) (bool, error) {
	if !s.mode.IsInteractive() {
		return def, nil
	}
	for {
		if err := s.term.Write(heading(s.style, prompt) + " " + confirmSuffix(def) + ": "); err != nil {
			return false, err
		}
		if err := s.term.Flush(); err != nil {
			return false, err
		}
		line, ok, err := s.term.ReadLine()
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
			if err := notice(s.term, s.style, "please answer 'y' or 'n'"); err != nil {
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
