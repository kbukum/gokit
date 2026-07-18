package viz

import (
	"fmt"

	"github.com/kbukum/gokit/bench"
)

// RenderDistribution generates an SVG histogram of score distributions.
// Each ScoreDistribution represents one label's score histogram.
func RenderDistribution(dists []bench.ScoreDistribution, opts ...RenderOption) string {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	s := newSVG(cfg.width, cfg.height)

	const (
		padLeft   = 60
		padTop    = 50
		padRight  = 20
		padBottom = 50
	)

	plotW := float64(cfg.width - padLeft - padRight)
	plotH := float64(cfg.height - padTop - padBottom)

	// Title.
	s.text(float64(cfg.width)/2, 22, "Score Distribution", "#333", 16, `text-anchor="middle" font-weight="bold"`)

	// Axes.
	drawAxes(s, padTop, plotW, plotH)

	// Axis labels.
	s.text(float64(padLeft)+plotW/2, float64(cfg.height)-8, "Score", "#555", 12, `text-anchor="middle"`)
	s.text(14, float64(padTop)+plotH/2, "Count", "#555", 12,
		fmt.Sprintf(`text-anchor="middle" transform="rotate(-90, 14, %.0f)"`, float64(padTop)+plotH/2))

	// Find global max count.
	maxCount := 1
	for _, d := range dists {
		for _, c := range d.Counts {
			if c > maxCount {
				maxCount = c
			}
		}
	}

	// Determine number of bins from the first distribution.
	numBins := 0
	for _, d := range dists {
		if len(d.Counts) > numBins {
			numBins = len(d.Counts)
		}
	}
	if numBins == 0 {
		return s.String()
	}

	// Bar width accounting for grouped bars per bin.
	groupCount := len(dists)
	binWidth := plotW / float64(numBins)
	barWidth := binWidth / float64(groupCount+1)

	// X-axis tick labels.
	for i := 0; i <= numBins; i++ {
		v := float64(i) / float64(numBins)
		x := float64(padLeft) + v*plotW
		s.text(x, float64(padTop)+plotH+18, fmt.Sprintf("%.1f", v), "#666", 10, `text-anchor="middle"`)
	}

	// Y-axis tick labels.
	steps := 4
	for i := 0; i <= steps; i++ {
		v := float64(i) / float64(steps) * float64(maxCount)
		y := float64(padTop) + plotH - (float64(i)/float64(steps))*plotH
		s.text(float64(padLeft)-8, y+4, fmt.Sprintf("%.0f", v), "#666", 10, `text-anchor="end"`)
		s.line(float64(padLeft), y, float64(padLeft)+plotW, y, "#eee", 0.5)
	}

	// Draw bars.
	for di, d := range dists {
		color := colorAt(di)
		for bi, count := range d.Counts {
			if count == 0 {
				continue
			}
			barH := (float64(count) / float64(maxCount)) * plotH
			x := float64(padLeft) + float64(bi)*binWidth + float64(di)*barWidth + barWidth*0.1
			y := float64(padTop) + plotH - barH
			s.rectF(x, y, barWidth*0.8, barH, color, `opacity="0.75"`)
		}
	}

	// Legend.
	for i, d := range dists {
		lx := float64(padLeft) + 10
		ly := float64(padTop) + 10 + float64(i)*18
		s.rectF(lx, ly-10, 12, 12, colorAt(i))
		s.text(lx+16, ly, d.Label, "#333", 11, "")
	}

	return s.String()
}
