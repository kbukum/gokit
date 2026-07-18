package render

import (
	"strings"
	"unicode/utf8"

	"github.com/kbukum/gokit/cli/theme"
)

// tableBorder is the rune set used to draw a table's frame.
type tableBorder struct {
	horizontal, vertical   string
	topLeft, topMid, topRt string
	midLeft, midMid, midRt string
	botLeft, botMid, botRt string
}

// unicodeBorder draws box-drawing frames; asciiBorder is the byte-clean fallback
// for terminals that cannot display Unicode.
func unicodeBorder() tableBorder {
	return tableBorder{
		horizontal: "─", vertical: "│",
		topLeft: "┌", topMid: "┬", topRt: "┐",
		midLeft: "├", midMid: "┼", midRt: "┤",
		botLeft: "└", botMid: "┴", botRt: "┘",
	}
}

func asciiBorder() tableBorder {
	return tableBorder{
		horizontal: "-", vertical: "|",
		topLeft: "+", topMid: "+", topRt: "+",
		midLeft: "+", midMid: "+", midRt: "+",
		botLeft: "+", botMid: "+", botRt: "+",
	}
}

// OutputTable is a formatted table for terminal output.
//
// The zero value is not usable; construct one with [NewOutputTable]. Rendering is pure —
// [OutputTable.String] builds the whole table as a string — so callers choose where to write it.
// Borders default to Unicode box-drawing;
// pass an ASCII-only [theme.Glyphs] via [OutputTable.WithGlyphs] to stay byte-clean on non-UTF-8 terminals.
type OutputTable struct {
	title   string
	columns []string
	rows    [][]string
	border  tableBorder
}

// NewOutputTable creates a table with the given column headings.
func NewOutputTable(columns ...string) *OutputTable {
	cols := make([]string, len(columns))
	copy(cols, columns)
	return &OutputTable{columns: cols, border: unicodeBorder()}
}

// WithGlyphs selects the border charset from a glyph set's Unicode capability:
// box-drawing when Unicode is enabled, ASCII (+-|) otherwise.
func (t *OutputTable) WithGlyphs(glyphs theme.Glyphs) *OutputTable {
	if glyphs.Unicode() {
		t.border = unicodeBorder()
	} else {
		t.border = asciiBorder()
	}
	return t
}

// WithTitle sets a human-readable table title and returns the receiver.
func (t *OutputTable) WithTitle(title string) *OutputTable {
	t.title = title
	return t
}

// AddRow appends a row of cell values.
//
// The row is normalized to the column count: extra cells are dropped
// and missing cells are padded with empty strings, so every rendered row lines up with the header
// and borders regardless of the caller's cell count.
func (t *OutputTable) AddRow(cells ...string) *OutputTable {
	row := make([]string, len(t.columns))
	copy(row, cells)
	t.rows = append(t.rows, row)
	return t
}

// String renders the table as a bordered, column-aligned block.
func (t *OutputTable) String() string {
	widths := make([]int, len(t.columns))
	for i, c := range t.columns {
		widths[i] = utf8.RuneCountInString(c)
	}
	for _, row := range t.rows {
		for i, cell := range row {
			if i < len(widths) {
				widths[i] = max(widths[i], utf8.RuneCountInString(cell))
			}
		}
	}

	var b strings.Builder
	if t.title != "" {
		b.WriteByte('\n')
		b.WriteString(t.title)
		b.WriteByte('\n')
	}

	b.WriteString(t.borderLine(widths, t.border.topLeft, t.border.topMid, t.border.topRt))
	b.WriteByte('\n')
	b.WriteString(t.cellsLine(t.columns, widths))
	b.WriteByte('\n')
	b.WriteString(t.borderLine(widths, t.border.midLeft, t.border.midMid, t.border.midRt))
	b.WriteByte('\n')
	for _, row := range t.rows {
		b.WriteString(t.cellsLine(row, widths))
		b.WriteByte('\n')
	}
	b.WriteString(t.borderLine(widths, t.border.botLeft, t.border.botMid, t.border.botRt))
	return b.String()
}

// borderLine builds a horizontal rule with the given corner/junction runes.
func (t *OutputTable) borderLine(widths []int, left, mid, right string) string {
	segments := make([]string, len(widths))
	for i, w := range widths {
		segments[i] = strings.Repeat(t.border.horizontal, w+2)
	}
	return left + strings.Join(segments, mid) + right
}

// cellsLine renders one padded, vertical-bar-separated row.
func (t *OutputTable) cellsLine(cells []string, widths []int) string {
	parts := make([]string, len(widths))
	for i := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		pad := widths[i] - utf8.RuneCountInString(cell)
		if pad < 0 {
			pad = 0
		}
		parts[i] = " " + cell + strings.Repeat(" ", pad) + " "
	}
	return t.border.vertical + strings.Join(parts, t.border.vertical) + t.border.vertical
}
