package viz

import (
	"fmt"
	"sort"

	"github.com/kbukum/gokit/bench"
)

// RenderComparison generates an SVG grouped bar chart comparing
// branches across their key metrics.
func RenderComparison(branches map[string]bench.BranchResult, opts ...RenderOption) string {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	s := newSVG(cfg.width, cfg.height)

	const (
		padLeft   = 60
		padTop    = 50
		padRight  = 20
		padBottom = 70
	)

	plotW := float64(cfg.width - padLeft - padRight)
	plotH := float64(cfg.height - padTop - padBottom)

	// Title.
	s.text(float64(cfg.width)/2, 22, "Branch Comparison", "#333", 16, `text-anchor="middle" font-weight="bold"`)

	// Collect and sort branch names.
	branchNames := make([]string, 0, len(branches))
	for name := range branches {
		branchNames = append(branchNames, name)
	}
	sort.Strings(branchNames)

	// Collect the union of metric names.
	metricSet := make(map[string]struct{})
	for _, br := range branches {
		for m := range br.Metrics {
			metricSet[m] = struct{}{}
		}
	}
	metricNames := make([]string, 0, len(metricSet))
	for m := range metricSet {
		metricNames = append(metricNames, m)
	}
	sort.Strings(metricNames)

	if len(branchNames) == 0 || len(metricNames) == 0 {
		return s.String()
	}

	// Axes.
	drawAxes(s, padLeft, padTop, plotW, plotH)

	// Y-axis label.
	s.text(14, float64(padTop)+plotH/2, "Value", "#555", 12,
		fmt.Sprintf(`text-anchor="middle" transform="rotate(-90, 14, %.0f)"`, float64(padTop)+plotH/2))

	// Find max value for scaling.
	maxVal := 1.0
	for _, br := range branches {
		for _, v := range br.Metrics {
			if v > maxVal {
				maxVal = v
			}
		}
	}

	// Y-axis ticks.
	steps := 4
	for i := 0; i <= steps; i++ {
		v := float64(i) / float64(steps) * maxVal
		y := float64(padTop) + plotH - (float64(i)/float64(steps))*plotH
		s.text(float64(padLeft)-8, y+4, fmt.Sprintf("%.2f", v), "#666", 10, `text-anchor="end"`)
		s.line(float64(padLeft), y, float64(padLeft)+plotW, y, "#eee", 0.5)
	}

	// Layout: groups of branches, each bar = one metric.
	nBranches := len(branchNames)
	nMetrics := len(metricNames)
	groupWidth := plotW / float64(nBranches)
	barWidth := groupWidth / float64(nMetrics+1)

	for bi, bName := range branchNames {
		br := branches[bName]
		groupX := float64(padLeft) + float64(bi)*groupWidth

		// Branch label.
		s.text(groupX+groupWidth/2, float64(padTop)+plotH+18, bName, "#333", 11,
			`text-anchor="middle"`)

		for mi, mName := range metricNames {
			v := br.Metrics[mName]
			barH := (v / maxVal) * plotH
			x := groupX + float64(mi)*barWidth + barWidth*0.15
			y := float64(padTop) + plotH - barH
			s.rectF(x, y, barWidth*0.7, barH, colorAt(mi), `opacity="0.85"`)
		}
	}

	// Legend for metrics.
	for i, mName := range metricNames {
		lx := float64(padLeft) + 10
		ly := float64(padTop) + 10 + float64(i)*18
		s.rectF(lx, ly-10, 12, 12, colorAt(i))
		s.text(lx+16, ly, mName, "#333", 11, "")
	}

	return s.String()
}
