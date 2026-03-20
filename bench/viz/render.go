package viz

import (
	"encoding/json"
	"sort"

	"github.com/kbukum/gokit/bench"
)

// renderConfig holds rendering settings.
type renderConfig struct {
	width  int
	height int
}

func defaultConfig() renderConfig {
	return renderConfig{width: 600, height: 400}
}

// RenderOption configures rendering.
type RenderOption func(*renderConfig)

// WithSize sets the SVG dimensions.
func WithSize(w, h int) RenderOption {
	return func(c *renderConfig) {
		c.width = w
		c.height = h
	}
}

// RenderAll generates SVG visualizations from run results.
// Returns a map of filename → SVG content. Only charts whose
// prerequisite data exists in the result are included.
func RenderAll(result *bench.RunResult, opts ...RenderOption) map[string]string {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	out := make(map[string]string)

	// Confusion matrix from metric details.
	if cm := extractConfusionMatrix(result); cm != nil {
		out["confusion_matrix.svg"] = RenderConfusion(cm, opts...)
	}

	// ROC curve from curves map.
	if roc := extractROC(result); roc != nil {
		out["roc_curve.svg"] = RenderROC(roc, opts...)
	}

	// Calibration curve.
	if cal := extractCalibration(result); cal != nil {
		out["calibration_curve.svg"] = RenderCalibration(cal, opts...)
	}

	// Score distribution from samples.
	if dists := extractDistributions(result); len(dists) > 0 {
		out["score_distribution.svg"] = RenderDistribution(dists, opts...)
	}

	// Branch comparison.
	if len(result.Branches) > 0 {
		out["branch_comparison.svg"] = RenderComparison(result.Branches, opts...)
	}

	return out
}

// --- extraction helpers ---

func extractConfusionMatrix(r *bench.RunResult) *bench.ConfusionMatrixDetail {
	// Search metric details for a ConfusionMatrixDetail.
	for _, m := range r.Metrics {
		if cm := decodeAs[bench.ConfusionMatrixDetail](m.Detail); cm != nil {
			return cm
		}
	}
	// Also check curves map.
	if raw, ok := r.Curves["confusion_matrix"]; ok {
		return decodeAs[bench.ConfusionMatrixDetail](raw)
	}
	return nil
}

func extractROC(r *bench.RunResult) *bench.ROCCurve {
	if raw, ok := r.Curves["roc"]; ok {
		return decodeAs[bench.ROCCurve](raw)
	}
	for _, m := range r.Metrics {
		if roc := decodeAs[bench.ROCCurve](m.Detail); roc != nil {
			return roc
		}
	}
	return nil
}

func extractCalibration(r *bench.RunResult) *bench.CalibrationCurve {
	if raw, ok := r.Curves["calibration"]; ok {
		return decodeAs[bench.CalibrationCurve](raw)
	}
	for _, m := range r.Metrics {
		if cal := decodeAs[bench.CalibrationCurve](m.Detail); cal != nil {
			return cal
		}
	}
	return nil
}

func extractDistributions(r *bench.RunResult) []bench.ScoreDistribution {
	// Check curves map.
	if raw, ok := r.Curves["score_distribution"]; ok {
		if dists := decodeAs[[]bench.ScoreDistribution](raw); dists != nil {
			return *dists
		}
	}
	// Build from samples if available.
	if len(r.Samples) == 0 {
		return nil
	}
	return buildDistributions(r.Samples)
}

// buildDistributions creates score distributions from sample results.
func buildDistributions(samples []bench.SampleResult) []bench.ScoreDistribution {
	const numBins = 10

	byLabel := make(map[string][]float64)
	for _, s := range samples {
		byLabel[s.Label] = append(byLabel[s.Label], s.Score)
	}

	labels := make([]string, 0, len(byLabel))
	for l := range byLabel {
		labels = append(labels, l)
	}
	sort.Strings(labels)

	var dists []bench.ScoreDistribution
	for _, label := range labels {
		scores := byLabel[label]
		bins := make([]float64, numBins+1)
		for i := range bins {
			bins[i] = float64(i) / float64(numBins)
		}
		counts := make([]int, numBins)
		for _, sc := range scores {
			idx := int(sc * float64(numBins))
			if idx >= numBins {
				idx = numBins - 1
			}
			if idx < 0 {
				idx = 0
			}
			counts[idx]++
		}
		dists = append(dists, bench.ScoreDistribution{
			Label:  label,
			Bins:   bins,
			Counts: counts,
		})
	}
	return dists
}

// decodeAs attempts to convert v into type T.
// It handles the case where v is already T, *T, or a JSON-serialised
// map/slice that can be marshalled then unmarshalled into T.
func decodeAs[T any](v any) *T {
	if v == nil {
		return nil
	}
	// Direct type match.
	if t, ok := v.(T); ok {
		return &t
	}
	if t, ok := v.(*T); ok {
		return t
	}
	// Re-marshal through JSON for map[string]any round-trip cases.
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var t T
	if err := json.Unmarshal(b, &t); err != nil {
		return nil
	}
	return &t
}
