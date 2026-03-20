package viz

import (
	"fmt"

	"github.com/kbukum/gokit/bench"
)

// RenderConfusion generates an SVG heatmap for a confusion matrix.
func RenderConfusion(cm *bench.ConfusionMatrixDetail, opts ...RenderOption) string {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	s := newSVG(cfg.width, cfg.height)

	n := len(cm.Labels)
	if n == 0 {
		return s.String()
	}

	// Layout constants.
	const (
		padLeft   = 90
		padTop    = 60
		padRight  = 20
		padBottom = 60
	)

	plotW := cfg.width - padLeft - padRight
	plotH := cfg.height - padTop - padBottom
	cellW := float64(plotW) / float64(n)
	cellH := float64(plotH) / float64(n)

	// Find max value for color scaling.
	maxVal := 1
	for _, row := range cm.Matrix {
		for _, v := range row {
			if v > maxVal {
				maxVal = v
			}
		}
	}

	// Title.
	s.text(float64(cfg.width)/2, 25, "Confusion Matrix", "#333", 16, `text-anchor="middle" font-weight="bold"`)

	// Axis labels.
	s.text(float64(cfg.width)/2, float64(cfg.height)-8, "Predicted", "#555", 12, `text-anchor="middle"`)
	s.text(14, float64(cfg.height)/2, "Actual", "#555", 12,
		fmt.Sprintf(`text-anchor="middle" transform="rotate(-90, 14, %.0f)"`, float64(cfg.height)/2))

	// Draw cells.
	for r, row := range cm.Matrix {
		for c, v := range row {
			x := float64(padLeft) + float64(c)*cellW
			y := float64(padTop) + float64(r)*cellH
			intensity := float64(v) / float64(maxVal)
			fill := heatColor(intensity)
			s.rectF(x, y, cellW, cellH, fill, `stroke="white" stroke-width="2"`)

			// Value text — use white on dark cells.
			textColor := "#333"
			if intensity > 0.5 {
				textColor = "white"
			}
			s.text(x+cellW/2, y+cellH/2+5, fmt.Sprintf("%d", v), textColor, 14, `text-anchor="middle"`)
		}
	}

	// Column labels (predicted).
	for i, label := range cm.Labels {
		x := float64(padLeft) + float64(i)*cellW + cellW/2
		s.text(x, float64(padTop)-8, label, "#333", 11, `text-anchor="middle"`)
	}

	// Row labels (actual).
	for i, label := range cm.Labels {
		y := float64(padTop) + float64(i)*cellH + cellH/2 + 4
		s.text(float64(padLeft)-8, y, label, "#333", 11, `text-anchor="end"`)
	}

	return s.String()
}

// heatColor returns a color interpolated from light blue to dark blue.
func heatColor(t float64) string {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	// Interpolate from #E3F2FD (light) to #1565C0 (dark).
	r := int(227 - t*114) // 227 → 113
	g := int(242 - t*141) // 242 → 101
	b := int(253 - t*61)  // 253 → 192
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}
