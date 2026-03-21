package viz

import (
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/bench"
)

func makeTestResult() *bench.RunResult {
	return &bench.RunResult{
		ID:        "run-viz-001",
		Schema:    bench.SchemaVersion,
		Timestamp: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
		Tag:       "viz-test",
		Duration:  3 * time.Second,
		Dataset: bench.DatasetInfo{
			Name:              "eval-dataset",
			Version:           "2.0",
			SampleCount:       5,
			LabelDistribution: map[string]int{"positive": 3, "negative": 2},
		},
		Metrics: []bench.MetricResult{
			{
				Name:  "f1",
				Value: 0.88,
				Values: map[string]float64{
					"positive": 0.90,
					"negative": 0.85,
				},
				Detail: &bench.ConfusionMatrixDetail{
					Labels: []string{"positive", "negative"},
					Matrix: [][]int{
						{40, 5},
						{3, 22},
					},
					Orientation: "row=actual, col=predicted",
				},
			},
			{
				Name:  "auc",
				Value: 0.92,
			},
		},
		Branches: map[string]bench.BranchResult{
			"keyword": {
				Name:             "keyword",
				Tier:             1,
				Metrics:          map[string]float64{"f1": 0.80, "precision": 0.85},
				AvgScorePositive: 0.88,
				AvgScoreNegative: 0.25,
				Duration:         2 * time.Second,
				Errors:           0,
			},
			"semantic": {
				Name:             "semantic",
				Tier:             2,
				Metrics:          map[string]float64{"f1": 0.92, "precision": 0.90},
				AvgScorePositive: 0.95,
				AvgScoreNegative: 0.15,
				Duration:         3 * time.Second,
				Errors:           1,
			},
		},
		Samples: []bench.SampleResult{
			{ID: "s1", Label: "positive", Predicted: "positive", Score: 0.95, Correct: true, Duration: 100 * time.Millisecond},
			{ID: "s2", Label: "positive", Predicted: "positive", Score: 0.82, Correct: true, Duration: 120 * time.Millisecond},
			{ID: "s3", Label: "positive", Predicted: "negative", Score: 0.45, Correct: false, Duration: 110 * time.Millisecond},
			{ID: "s4", Label: "negative", Predicted: "negative", Score: 0.88, Correct: true, Duration: 90 * time.Millisecond},
			{ID: "s5", Label: "negative", Predicted: "positive", Score: 0.55, Correct: false, Duration: 95 * time.Millisecond},
		},
		Curves: map[string]any{
			"roc": bench.ROCCurve{
				FPR:        []float64{0.0, 0.1, 0.3, 0.5, 1.0},
				TPR:        []float64{0.0, 0.5, 0.7, 0.9, 1.0},
				Thresholds: []float64{1.0, 0.8, 0.5, 0.3, 0.0},
				AUC:        0.92,
			},
			"calibration": bench.CalibrationCurve{
				PredictedProbability: []float64{0.1, 0.3, 0.5, 0.7, 0.9},
				ActualFrequency:      []float64{0.12, 0.28, 0.52, 0.68, 0.91},
				BinCount:             []int{10, 12, 15, 11, 8},
			},
		},
	}
}

// --- RenderAll ---

func TestRenderAll(t *testing.T) {
	t.Parallel()
	result := makeTestResult()
	svgs := RenderAll(result)

	expectedKeys := []string{
		"confusion_matrix.svg",
		"roc_curve.svg",
		"calibration_curve.svg",
		"score_distribution.svg",
		"branch_comparison.svg",
	}

	for _, key := range expectedKeys {
		svg, ok := svgs[key]
		if !ok {
			t.Errorf("missing expected SVG %q", key)
			continue
		}
		if !strings.HasPrefix(svg, "<svg") {
			t.Errorf("%s: does not start with <svg, got prefix: %q", key, svg[:min(50, len(svg))])
		}
		if !strings.HasSuffix(svg, "</svg>") {
			t.Errorf("%s: does not end with </svg>", key)
		}
	}
}

func TestRenderAllWithSize(t *testing.T) {
	t.Parallel()
	result := makeTestResult()
	svgs := RenderAll(result, WithSize(800, 600))

	for key, svg := range svgs {
		if !strings.Contains(svg, `width="800"`) {
			t.Errorf("%s: expected width=800", key)
		}
		if !strings.Contains(svg, `height="600"`) {
			t.Errorf("%s: expected height=600", key)
		}
	}
}

func TestRenderAllEmpty(t *testing.T) {
	t.Parallel()
	result := &bench.RunResult{
		ID:        "empty-run",
		Timestamp: time.Now(),
	}
	svgs := RenderAll(result)

	if len(svgs) != 0 {
		t.Errorf("expected 0 SVGs for empty result, got %d: %v", len(svgs), keys(svgs))
	}
}

// --- Individual renderers ---

func TestRenderConfusion(t *testing.T) {
	t.Parallel()
	cm := &bench.ConfusionMatrixDetail{
		Labels: []string{"positive", "negative"},
		Matrix: [][]int{
			{40, 5},
			{3, 22},
		},
	}

	svg := RenderConfusion(cm)
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("output is not an SVG")
	}
	if !strings.Contains(svg, "Confusion Matrix") {
		t.Error("missing title")
	}
	if !strings.Contains(svg, "positive") {
		t.Error("missing label text")
	}
	if !strings.Contains(svg, ">40<") {
		t.Error("missing cell value 40")
	}
	if !strings.Contains(svg, ">22<") {
		t.Error("missing cell value 22")
	}
}

func TestRenderConfusionEmpty(t *testing.T) {
	t.Parallel()
	cm := &bench.ConfusionMatrixDetail{Labels: []string{}}
	svg := RenderConfusion(cm)

	if !strings.HasPrefix(svg, "<svg") {
		t.Error("empty confusion matrix should still produce valid SVG")
	}
	// Should not have any rect cells (just the background)
	if strings.Contains(svg, "Actual") {
		t.Error("empty confusion matrix should not have axis labels")
	}
}

func TestRenderConfusionWithSize(t *testing.T) {
	t.Parallel()
	cm := &bench.ConfusionMatrixDetail{
		Labels: []string{"a", "b"},
		Matrix: [][]int{{10, 2}, {3, 15}},
	}
	svg := RenderConfusion(cm, WithSize(300, 300))
	if !strings.Contains(svg, `width="300"`) {
		t.Error("expected width=300")
	}
}

func TestRenderROC(t *testing.T) {
	t.Parallel()
	roc := &bench.ROCCurve{
		FPR:        []float64{0.0, 0.1, 0.3, 0.5, 1.0},
		TPR:        []float64{0.0, 0.5, 0.7, 0.9, 1.0},
		Thresholds: []float64{1.0, 0.8, 0.5, 0.3, 0.0},
		AUC:        0.92,
	}

	svg := RenderROC(roc)
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("output is not an SVG")
	}
	if !strings.Contains(svg, "ROC Curve") {
		t.Error("missing title")
	}
	if !strings.Contains(svg, "AUC") {
		t.Error("missing AUC annotation")
	}
	if !strings.Contains(svg, "0.92") {
		t.Error("missing AUC value")
	}
	// Should have polyline for the curve
	if !strings.Contains(svg, "<polyline") {
		t.Error("missing polyline for ROC curve")
	}
	// Diagonal reference line
	if !strings.Contains(svg, "stroke-dasharray") {
		t.Error("missing dashed diagonal reference line")
	}
}

func TestRenderROCEmpty(t *testing.T) {
	t.Parallel()
	roc := &bench.ROCCurve{FPR: []float64{}, TPR: []float64{}}
	svg := RenderROC(roc)

	if !strings.HasPrefix(svg, "<svg") {
		t.Error("empty ROC should still produce valid SVG")
	}
	// No polyline for empty data
	if strings.Contains(svg, "<polyline") {
		t.Error("empty ROC should not have polyline")
	}
}

func TestRenderCalibration(t *testing.T) {
	t.Parallel()
	cal := &bench.CalibrationCurve{
		PredictedProbability: []float64{0.1, 0.3, 0.5, 0.7, 0.9},
		ActualFrequency:      []float64{0.12, 0.28, 0.52, 0.68, 0.91},
		BinCount:             []int{10, 12, 15, 11, 8},
	}

	svg := RenderCalibration(cal)
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("output is not an SVG")
	}
	if !strings.Contains(svg, "Calibration Curve") {
		t.Error("missing title")
	}
	if !strings.Contains(svg, "<polyline") {
		t.Error("missing polyline for calibration curve")
	}
	// Should have data point circles
	if !strings.Contains(svg, "<circle") {
		t.Error("missing data point circles")
	}
	// Diagonal reference
	if !strings.Contains(svg, "stroke-dasharray") {
		t.Error("missing dashed diagonal reference line")
	}
}

func TestRenderCalibrationEmpty(t *testing.T) {
	t.Parallel()
	cal := &bench.CalibrationCurve{
		PredictedProbability: []float64{},
		ActualFrequency:      []float64{},
	}
	svg := RenderCalibration(cal)
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("empty calibration should still produce valid SVG")
	}
}

func TestRenderDistribution(t *testing.T) {
	t.Parallel()
	dists := []bench.ScoreDistribution{
		{
			Label:  "positive",
			Bins:   []float64{0.0, 0.2, 0.4, 0.6, 0.8, 1.0},
			Counts: []int{1, 3, 5, 8, 4},
		},
		{
			Label:  "negative",
			Bins:   []float64{0.0, 0.2, 0.4, 0.6, 0.8, 1.0},
			Counts: []int{7, 4, 2, 1, 0},
		},
	}

	svg := RenderDistribution(dists)
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("output is not an SVG")
	}
	if !strings.Contains(svg, "Score Distribution") {
		t.Error("missing title")
	}
	// Should have rect elements for bars
	if !strings.Contains(svg, "<rect") {
		t.Error("missing bar rectangles")
	}
	// Legend labels
	if !strings.Contains(svg, "positive") {
		t.Error("missing positive label in legend")
	}
	if !strings.Contains(svg, "negative") {
		t.Error("missing negative label in legend")
	}
}

func TestRenderDistributionEmpty(t *testing.T) {
	t.Parallel()
	dists := []bench.ScoreDistribution{
		{Label: "x", Bins: []float64{}, Counts: []int{}},
	}
	svg := RenderDistribution(dists)
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("empty distribution should still produce valid SVG")
	}
}

func TestRenderComparison(t *testing.T) {
	t.Parallel()
	branches := map[string]bench.BranchResult{
		"keyword": {
			Name:    "keyword",
			Tier:    1,
			Metrics: map[string]float64{"f1": 0.80, "precision": 0.85},
		},
		"semantic": {
			Name:    "semantic",
			Tier:    2,
			Metrics: map[string]float64{"f1": 0.92, "precision": 0.90},
		},
	}

	svg := RenderComparison(branches)
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("output is not an SVG")
	}
	if !strings.Contains(svg, "Branch Comparison") {
		t.Error("missing title")
	}
	if !strings.Contains(svg, "keyword") {
		t.Error("missing keyword branch label")
	}
	if !strings.Contains(svg, "semantic") {
		t.Error("missing semantic branch label")
	}
	// Should have rect elements for bars
	if !strings.Contains(svg, "<rect") {
		t.Error("missing bar rectangles")
	}
}

func TestRenderComparisonEmpty(t *testing.T) {
	t.Parallel()
	branches := map[string]bench.BranchResult{}
	svg := RenderComparison(branches)
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("empty comparison should still produce valid SVG")
	}
}

func TestRenderComparisonNoMetrics(t *testing.T) {
	t.Parallel()
	branches := map[string]bench.BranchResult{
		"empty": {Name: "empty", Metrics: map[string]float64{}},
	}
	svg := RenderComparison(branches)
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("comparison with empty metrics should still produce valid SVG")
	}
}

// --- SVG builder ---

func TestSVGBuilder(t *testing.T) {
	t.Parallel()
	s := newSVG(100, 50)

	if s.width != 100 || s.height != 50 {
		t.Errorf("dimensions = %dx%d, want 100x50", s.width, s.height)
	}

	s.rect(10, 10, 20, 20, "red")
	s.rectF(10.5, 10.5, 20.5, 20.5, "blue")
	s.line(0, 0, 100, 50, "black", 2)
	s.text(50, 25, "Hello", "green", 12)
	s.circle(50, 25, 5, "yellow")

	out := s.String()

	checks := []struct {
		name, needle string
	}{
		{"svg open", `<svg xmlns=`},
		{"svg close", `</svg>`},
		{"viewBox", `viewBox="0 0 100 50"`},
		{"background rect", `fill="white"`},
		{"int rect", `<rect x="10"`},
		{"float rect", `<rect x="10.50"`},
		{"line", `<line x1="0.00"`},
		{"text", `>Hello</text>`},
		{"circle", `<circle cx="50.00"`},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.needle) {
			t.Errorf("%s: output missing %q", c.name, c.needle)
		}
	}
}

func TestSVGPolyline(t *testing.T) {
	t.Parallel()
	s := newSVG(100, 100)
	pts := []point{{10, 20}, {30, 40}, {50, 60}}
	s.polyline(pts, "blue", 1.5)

	out := s.String()
	if !strings.Contains(out, "<polyline") {
		t.Error("missing polyline element")
	}
	if !strings.Contains(out, "10.00,20.00") {
		t.Error("missing first point")
	}
}

func TestSVGPolylineEmpty(t *testing.T) {
	t.Parallel()
	s := newSVG(100, 100)
	s.polyline(nil, "blue", 1)
	out := s.String()
	if strings.Contains(out, "<polyline") {
		t.Error("empty polyline should not add element")
	}
}

func TestXMLEscape(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"a & b", "a &amp; b"},
		{"<script>", "&lt;script&gt;"},
		{`say "hi"`, `say &quot;hi&quot;`},
	}
	for _, tt := range tests {
		got := xmlEscape(tt.input)
		if got != tt.want {
			t.Errorf("xmlEscape(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestColorAt(t *testing.T) {
	t.Parallel()
	// Should cycle through palette
	c0 := colorAt(0)
	c1 := colorAt(1)
	if c0 == c1 {
		t.Error("first two colors should be different")
	}
	// Should wrap around
	cWrap := colorAt(len(palette))
	if cWrap != c0 {
		t.Errorf("colorAt(%d) = %q, want %q (wrap to 0)", len(palette), cWrap, c0)
	}
}

func TestHeatColor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input float64
		check func(string) bool
		desc  string
	}{
		{0.0, func(s string) bool { return strings.HasPrefix(s, "#") && len(s) == 7 }, "valid hex"},
		{1.0, func(s string) bool { return strings.HasPrefix(s, "#") && len(s) == 7 }, "valid hex"},
		{-0.5, func(s string) bool { return s == heatColor(0.0) }, "clamped to 0"},
		{1.5, func(s string) bool { return s == heatColor(1.0) }, "clamped to 1"},
	}
	for _, tt := range tests {
		c := heatColor(tt.input)
		if !tt.check(c) {
			t.Errorf("heatColor(%v) = %q: %s failed", tt.input, c, tt.desc)
		}
	}

	// Light should differ from dark
	light := heatColor(0.0)
	dark := heatColor(1.0)
	if light == dark {
		t.Error("heatColor(0) and heatColor(1) should be different")
	}
}

// --- Extraction helpers ---

func TestExtractConfusionMatrix(t *testing.T) {
	t.Parallel()
	result := makeTestResult()
	cm := extractConfusionMatrix(result)
	if cm == nil {
		t.Fatal("expected non-nil ConfusionMatrixDetail")
	}
	if len(cm.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(cm.Labels))
	}
}

func TestExtractROC(t *testing.T) {
	t.Parallel()
	result := makeTestResult()
	roc := extractROC(result)
	if roc == nil {
		t.Fatal("expected non-nil ROCCurve")
	}
	if len(roc.FPR) != 5 {
		t.Errorf("expected 5 FPR points, got %d", len(roc.FPR))
	}
}

func TestExtractCalibration(t *testing.T) {
	t.Parallel()
	result := makeTestResult()
	cal := extractCalibration(result)
	if cal == nil {
		t.Fatal("expected non-nil CalibrationCurve")
	}
	if len(cal.PredictedProbability) != 5 {
		t.Errorf("expected 5 calibration points, got %d", len(cal.PredictedProbability))
	}
}

func TestExtractDistributions(t *testing.T) {
	t.Parallel()

	t.Run("from samples", func(t *testing.T) {
		t.Parallel()
		result := makeTestResult()
		result.Curves = nil // Force building from samples
		dists := extractDistributions(result)
		if len(dists) == 0 {
			t.Fatal("expected distributions built from samples")
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		result := &bench.RunResult{}
		dists := extractDistributions(result)
		if dists != nil {
			t.Error("expected nil for empty result")
		}
	})
}

func TestDecodeAs(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		t.Parallel()
		result := decodeAs[bench.ROCCurve](nil)
		if result != nil {
			t.Error("expected nil for nil input")
		}
	})

	t.Run("direct type", func(t *testing.T) {
		t.Parallel()
		roc := bench.ROCCurve{AUC: 0.95}
		result := decodeAs[bench.ROCCurve](roc)
		if result == nil || result.AUC != 0.95 {
			t.Error("direct type match failed")
		}
	})

	t.Run("pointer type", func(t *testing.T) {
		t.Parallel()
		roc := &bench.ROCCurve{AUC: 0.95}
		result := decodeAs[bench.ROCCurve](roc)
		if result == nil || result.AUC != 0.95 {
			t.Error("pointer type match failed")
		}
	})

	t.Run("json roundtrip", func(t *testing.T) {
		t.Parallel()
		// Simulate a map[string]any as would come from JSON deserialization
		m := map[string]any{
			"Labels":      []any{"a", "b"},
			"Matrix":      []any{[]any{float64(1), float64(2)}, []any{float64(3), float64(4)}},
			"Orientation": "test",
		}
		result := decodeAs[bench.ConfusionMatrixDetail](m)
		if result == nil {
			t.Fatal("JSON roundtrip decode failed")
		}
		if len(result.Labels) != 2 {
			t.Errorf("expected 2 labels, got %d", len(result.Labels))
		}
	})
}

func TestClamp01(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want float64
	}{
		{-1.0, 0.0},
		{0.0, 0.0},
		{0.5, 0.5},
		{1.0, 1.0},
		{2.0, 1.0},
	}
	for _, tt := range tests {
		got := clamp01(tt.in)
		if got != tt.want {
			t.Errorf("clamp01(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// --- helpers ---

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
