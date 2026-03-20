package viz

import (
	"fmt"

	"github.com/kbukum/gokit/bench"
)

// RenderROC generates an SVG plot of a receiver operating characteristic curve.
func RenderROC(roc *bench.ROCCurve, opts ...RenderOption) string {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	s := newSVG(cfg.width, cfg.height)

	const (
		padLeft   = 60
		padTop    = 40
		padRight  = 20
		padBottom = 50
	)

	plotW := float64(cfg.width - padLeft - padRight)
	plotH := float64(cfg.height - padTop - padBottom)

	// Title.
	s.text(float64(cfg.width)/2, 22, "ROC Curve", "#333", 16, `text-anchor="middle" font-weight="bold"`)

	// Axes.
	drawAxes(s, padLeft, padTop, plotW, plotH)

	// Axis labels.
	s.text(float64(padLeft)+plotW/2, float64(cfg.height)-8, "False Positive Rate", "#555", 12, `text-anchor="middle"`)
	s.text(14, float64(padTop)+plotH/2, "True Positive Rate", "#555", 12,
		fmt.Sprintf(`text-anchor="middle" transform="rotate(-90, 14, %.0f)"`, float64(padTop)+plotH/2))

	// Tick labels.
	for i := 0; i <= 4; i++ {
		v := float64(i) / 4.0
		x := float64(padLeft) + v*plotW
		y := float64(padTop) + plotH
		s.text(x, y+18, fmt.Sprintf("%.2f", v), "#666", 10, `text-anchor="middle"`)
		// Grid line.
		s.line(x, float64(padTop), x, y, "#eee", 0.5)
	}
	for i := 0; i <= 4; i++ {
		v := float64(i) / 4.0
		x := float64(padLeft)
		y := float64(padTop) + plotH - v*plotH
		s.text(x-8, y+4, fmt.Sprintf("%.2f", v), "#666", 10, `text-anchor="end"`)
		s.line(float64(padLeft), y, float64(padLeft)+plotW, y, "#eee", 0.5)
	}

	// Diagonal reference line (dashed).
	s.line(float64(padLeft), float64(padTop)+plotH,
		float64(padLeft)+plotW, float64(padTop),
		"#999", 1, `stroke-dasharray="6,4"`)

	// ROC curve.
	if len(roc.FPR) > 0 && len(roc.FPR) == len(roc.TPR) {
		pts := make([]point, len(roc.FPR))
		for i := range roc.FPR {
			pts[i] = point{
				x: float64(padLeft) + roc.FPR[i]*plotW,
				y: float64(padTop) + plotH - roc.TPR[i]*plotH,
			}
		}
		s.polyline(pts, "#4285F4", 2, "none")
	}

	// AUC annotation.
	s.text(float64(padLeft)+plotW-10, float64(padTop)+20,
		fmt.Sprintf("AUC = %.4f", roc.AUC), "#4285F4", 13, `text-anchor="end" font-weight="bold"`)

	return s.String()
}

// drawAxes draws the X and Y axis lines for a standard plot area.
func drawAxes(s *svg, padLeft, padTop int, plotW, plotH float64) {
	x0 := float64(padLeft)
	y0 := float64(padTop)
	// Y axis.
	s.line(x0, y0, x0, y0+plotH, "#333", 1.5)
	// X axis.
	s.line(x0, y0+plotH, x0+plotW, y0+plotH, "#333", 1.5)
}
