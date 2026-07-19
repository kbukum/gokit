package prompt

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kbukum/gokit/cli/theme"
)

// heading returns the bold heading line shown above a prompt's choices or input.
func heading(style theme.Style, prompt string) string {
	return style.Palette().Bold(prompt)
}

// decorate renders a choice label plus its annotation and the recommended marker.
func decorate(style theme.Style, choice Choice) string {
	var b strings.Builder
	b.WriteString(choice.Label())
	if choice.Annotation() != "" {
		b.WriteByte(' ')
		b.WriteString(style.Palette().Dim(choice.Annotation()))
	}
	if choice.IsRecommended() {
		b.WriteByte(' ')
		b.WriteString(style.Palette().Info("(recommended)"))
	}
	return b.String()
}

// numberedRows builds the static, one-based choice list a line-driven terminal prints.
func numberedRows(style theme.Style, choices []Choice) []string {
	rows := make([]string, len(choices))
	for i, choice := range choices {
		rows[i] = fmt.Sprintf("  %d) %s", i+1, decorate(style, choice))
	}
	return rows
}

// writeAnswer writes the inline answer marker ("  » [hint]: ") and flushes.
func writeAnswer(term Terminal, style theme.Style, hint string) error {
	marker := style.Glyphs().Answer()
	text := "  " + marker + " "
	if hint != "" {
		text = "  " + marker + " " + hint + ": "
	}
	if err := term.Write(text); err != nil {
		return err
	}
	return term.Flush()
}

// notice writes a warning notice line beneath a line prompt.
func notice(term Terminal, style theme.Style, text string) error {
	return term.WriteLine("  " + style.Palette().Warn(text))
}

// parseIndex parses a one-based choice number into a zero-based index within [0, length);
// the second return value is false when out of range or unparsable.
func parseIndex(input string, length int) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || n < 1 || n > length {
		return 0, false
	}
	return n - 1, true
}
