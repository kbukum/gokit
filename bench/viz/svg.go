package viz

import (
	"fmt"
	"strings"
)

// svg is a lightweight builder for constructing SVG documents.
type svg struct {
	width, height int
	elements      []string
}

func newSVG(w, h int) *svg {
	return &svg{width: w, height: h}
}

func (s *svg) rect(x, y, w, h int, fill string, attrs ...string) {
	extra := strings.Join(attrs, " ")
	el := fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="%s" %s/>`, x, y, w, h, fill, extra) //nolint:gocritic // SVG attribute quoting
	s.elements = append(s.elements, el)
}

func (s *svg) rectF(x, y, w, h float64, fill string, attrs ...string) {
	extra := strings.Join(attrs, " ")
	el := fmt.Sprintf(`<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" fill="%s" %s/>`, x, y, w, h, fill, extra) //nolint:gocritic // SVG attribute quoting
	s.elements = append(s.elements, el)
}

func (s *svg) line(x1, y1, x2, y2 float64, stroke string, strokeWidth float64, attrs ...string) {
	extra := strings.Join(attrs, " ")
	el := fmt.Sprintf(`<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" stroke="%s" stroke-width="%.1f" %s/>`, //nolint:gocritic // SVG attribute quoting
		x1, y1, x2, y2, stroke, strokeWidth, extra)
	s.elements = append(s.elements, el)
}

func (s *svg) text(x, y float64, content, fill string, fontSize int, attrs ...string) {
	extra := strings.Join(attrs, " ")
	el := fmt.Sprintf(`<text x="%.2f" y="%.2f" fill="%s" font-size="%d" font-family="sans-serif" %s>%s</text>`, //nolint:gocritic // SVG attribute quoting
		x, y, fill, fontSize, extra, xmlEscape(content))
	s.elements = append(s.elements, el)
}

func (s *svg) circle(cx, cy, r float64, fill string, attrs ...string) {
	extra := strings.Join(attrs, " ")
	el := fmt.Sprintf(`<circle cx="%.2f" cy="%.2f" r="%.2f" fill="%s" %s/>`, cx, cy, r, fill, extra) //nolint:gocritic // SVG attribute quoting
	s.elements = append(s.elements, el)
}

func (s *svg) polyline(points []point, stroke string, strokeWidth float64) {
	if len(points) == 0 {
		return
	}
	var b strings.Builder
	for i, p := range points {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%.2f,%.2f", p.x, p.y)
	}
	el := fmt.Sprintf(`<polyline points="%s" stroke="%s" stroke-width="%.1f" fill="none"/>`, //nolint:gocritic // SVG attribute quoting
		b.String(), stroke, strokeWidth)
	s.elements = append(s.elements, el)
}

func (s *svg) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		s.width, s.height, s.width, s.height)
	b.WriteByte('\n')
	// white background
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="white"/>`, s.width, s.height)
	b.WriteByte('\n')
	for _, e := range s.elements {
		b.WriteString(e)
		b.WriteByte('\n')
	}
	b.WriteString("</svg>")
	return b.String()
}

// point is a 2D coordinate.
type point struct {
	x, y float64
}

// xmlEscape escapes XML special characters.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// palette provides a set of distinct colors for chart elements.
var palette = []string{
	"#4285F4", // blue
	"#EA4335", // red
	"#34A853", // green
	"#FBBC05", // yellow
	"#9C27B0", // purple
	"#FF6D00", // orange
	"#00BCD4", // cyan
	"#795548", // brown
}

func colorAt(i int) string {
	return palette[i%len(palette)]
}
