package render

import (
	"strings"
	"unicode/utf8"
)

// OutputTable is a formatted table for terminal output.
//
// The zero value is not usable; construct one with [NewOutputTable]. Rendering
// is pure — [OutputTable.String] builds the whole table as a string — so callers
// choose where to write it.
type OutputTable struct {
	title   string
	columns []string
	rows    [][]string
}

// NewOutputTable creates a table with the given column headings.
func NewOutputTable(columns ...string) *OutputTable {
	cols := make([]string, len(columns))
	copy(cols, columns)
	return &OutputTable{columns: cols}
}

// WithTitle sets a human-readable table title and returns the receiver.
func (t *OutputTable) WithTitle(title string) *OutputTable {
	t.title = title
	return t
}

// AddRow appends a row of cell values.
//
// The row is normalized to the column count: extra cells are dropped and
// missing cells are padded with empty strings, so every rendered row lines up
// with the header and borders regardless of the caller's cell count.
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

	b.WriteString(border(widths, "┌", "┬", "┐"))
	b.WriteByte('\n')
	b.WriteString(cellsLine(t.columns, widths))
	b.WriteByte('\n')
	b.WriteString(border(widths, "├", "┼", "┤"))
	b.WriteByte('\n')
	for _, row := range t.rows {
		b.WriteString(cellsLine(row, widths))
		b.WriteByte('\n')
	}
	b.WriteString(border(widths, "└", "┴", "┘"))
	return b.String()
}

// border builds a horizontal rule with the given corner/junction runes.
func border(widths []int, left, mid, right string) string {
	segments := make([]string, len(widths))
	for i, w := range widths {
		segments[i] = strings.Repeat("─", w+2)
	}
	return left + strings.Join(segments, mid) + right
}

// cellsLine renders one padded, pipe-separated row.
func cellsLine(cells []string, widths []int) string {
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
	return "│" + strings.Join(parts, "│") + "│"
}
