package viz

import (
	"fmt"

	"github.com/kbukum/gokit/bench"
)

// RenderCalibration generates an SVG plot of a calibration curve.
func RenderCalibration(cal *bench.CalibrationCurve, opts ...RenderOption) string {
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
	s.text(float64(cfg.width)/2, 22, "Calibration Curve", "#333", 16, `text-anchor="middle" font-weight="bold"`)

	// Axes.
	drawAxes(s, padLeft, padTop, plotW, plotH)

	// Axis labels.
	s.text(float64(padLeft)+plotW/2, float64(cfg.height)-8, "Predicted Probability", "#555", 12, `text-anchor="middle"`)
	s.text(14, float64(padTop)+plotH/2, "Actual Frequency", "#555", 12,
		fmt.Sprintf(`text-anchor="middle" transform="rotate(-90, 14, %.0f)"`, float64(padTop)+plotH/2))

	// Tick labels and grid.
	for i := 0; i <= 4; i++ {
		v := float64(i) / 4.0
		x := float64(padLeft) + v*plotW
		y := float64(padTop) + plotH
		s.text(x, y+18, fmt.Sprintf("%.2f", v), "#666", 10, `text-anchor="middle"`)
		s.line(x, float64(padTop), x, y, "#eee", 0.5)
	}
	for i := 0; i <= 4; i++ {
		v := float64(i) / 4.0
		x := float64(padLeft)
		y := float64(padTop) + plotH - v*plotH
		s.text(x-8, y+4, fmt.Sprintf("%.2f", v), "#666", 10, `text-anchor="end"`)
		s.line(float64(padLeft), y, float64(padLeft)+plotW, y, "#eee", 0.5)
	}

	// Diagonal reference (perfectly calibrated).
	s.line(float64(padLeft), float64(padTop)+plotH,
		float64(padLeft)+plotW, float64(padTop),
		"#999", 1, `stroke-dasharray="6,4"`)

	// Calibration curve.
	n := len(cal.PredictedProbability)
	if n > 0 && n == len(cal.ActualFrequency) {
		pts := make([]point, n)
		for i := range pts {
			pts[i] = point{
				x: float64(padLeft) + clamp01(cal.PredictedProbability[i])*plotW,
				y: float64(padTop) + plotH - clamp01(cal.ActualFrequency[i])*plotH,
			}
		}
		s.polyline(pts, "#EA4335", 2, "none")

		// Draw data points.
		for _, p := range pts {
			s.circle(p.x, p.y, 3, "#EA4335")
		}
	}

	return s.String()
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
