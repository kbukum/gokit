package viz

import (
	"encoding/json"

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

// RenderAll generates SVG visualizations from run results. Returns a map of filename → SVG content.
// Only charts whose prerequisite data exists in the result are included.
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
	for metricIndex := range r.Metrics {
		m := &r.Metrics[metricIndex]
		if cm := m.Confusion; cm != nil {
			return cm
		}
	}
	// Also check curves map.
	return r.Confusion
}

func extractROC(r *bench.RunResult) *bench.ROCCurve {
	if r.ROC != nil {
		return r.ROC
	}
	for metricIndex := range r.Metrics {
		m := &r.Metrics[metricIndex]
		if roc := m.ROC; roc != nil {
			return roc
		}
	}
	return nil
}

func extractCalibration(r *bench.RunResult) *bench.CalibrationCurve {
	if r.Calibration != nil {
		return r.Calibration
	}
	for metricIndex := range r.Metrics {
		m := &r.Metrics[metricIndex]
		if cal := m.Calibration; cal != nil {
			return cal
		}
	}
	return nil
}

func extractDistributions(r *bench.RunResult) []bench.ScoreDistribution {
	// Check curves map.
	if len(r.ScoreDistributions) > 0 {
		return r.ScoreDistributions
	}
	return nil
}

// decodeAs attempts to convert v into type T. It handles the case where v is already T, *T,
// or a JSON-serialized map/slice that can be marshaled then unmarshaled into T.
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
