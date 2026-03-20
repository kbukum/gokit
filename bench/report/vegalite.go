package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/kbukum/gokit/bench"
)

const vegaLiteSchema = "https://vega.github.io/schema/vega-lite/v5.json"

// VegaLite creates a reporter that outputs Vega-Lite visualization specs.
// It generates multiple spec files as a JSON object { "filename": spec, ... }.
func VegaLite() Reporter {
	return &vegaLiteReporter{}
}

type vegaLiteReporter struct{}

func (r *vegaLiteReporter) Name() string { return "vegalite" }

func (r *vegaLiteReporter) Generate(w io.Writer, result *bench.RunResult) error {
	specs := VegaLiteSpecs(result)
	if len(specs) == 0 {
		_, err := io.WriteString(w, "{}\n")
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(specs)
}

// VegaLiteSpecs generates individual Vega-Lite specs from run results.
// Returns a map of filename → JSON spec.
func VegaLiteSpecs(result *bench.RunResult) map[string]any {
	specs := make(map[string]any)

	if spec := rocSpec(result); spec != nil {
		specs["roc_curve.vl.json"] = spec
	}
	if spec := confusionMatrixSpec(result); spec != nil {
		specs["confusion_matrix.vl.json"] = spec
	}
	if spec := thresholdSweepSpec(result); spec != nil {
		specs["threshold_sweep.vl.json"] = spec
	}
	if spec := calibrationSpec(result); spec != nil {
		specs["calibration.vl.json"] = spec
	}
	if spec := branchComparisonSpec(result); spec != nil {
		specs["branch_comparison.vl.json"] = spec
	}
	if spec := scoreDistributionSpec(result); spec != nil {
		specs["score_distribution.vl.json"] = spec
	}

	return specs
}

// findCurve looks for a named curve in the Curves map, then scans MetricResult.Detail
// for a matching type. Returns nil if not found.
func findCurve[T any](result *bench.RunResult, curveKey string) *T {
	if result.Curves != nil {
		if v, ok := result.Curves[curveKey]; ok {
			if typed, ok := v.(T); ok {
				return &typed
			}
			if typed, ok := v.(*T); ok {
				return typed
			}
		}
	}
	for _, m := range result.Metrics {
		if m.Detail == nil {
			continue
		}
		if typed, ok := m.Detail.(T); ok {
			return &typed
		}
		if typed, ok := m.Detail.(*T); ok {
			return typed
		}
	}
	return nil
}

// findAllCurves collects all instances of a type from Curves and MetricResult.Detail.
func findAllCurves[T any](result *bench.RunResult) []T {
	var out []T
	if result.Curves != nil {
		for _, v := range result.Curves {
			if typed, ok := v.(T); ok {
				out = append(out, typed)
			} else if typed, ok := v.(*T); ok {
				out = append(out, *typed)
			}
			// Also check slices stored in Curves.
			if sl, ok := v.([]T); ok {
				out = append(out, sl...)
			}
			if sl, ok := v.([]any); ok {
				for _, item := range sl {
					if typed, ok := item.(T); ok {
						out = append(out, typed)
					} else if typed, ok := item.(*T); ok {
						out = append(out, *typed)
					}
				}
			}
		}
	}
	for _, m := range result.Metrics {
		if m.Detail == nil {
			continue
		}
		if typed, ok := m.Detail.(T); ok {
			out = append(out, typed)
		} else if typed, ok := m.Detail.(*T); ok {
			out = append(out, *typed)
		}
		if sl, ok := m.Detail.([]T); ok {
			out = append(out, sl...)
		}
		if sl, ok := m.Detail.([]any); ok {
			for _, item := range sl {
				if typed, ok := item.(T); ok {
					out = append(out, typed)
				} else if typed, ok := item.(*T); ok {
					out = append(out, *typed)
				}
			}
		}
	}
	return out
}

func rocSpec(result *bench.RunResult) map[string]any {
	roc := findCurve[bench.ROCCurve](result, "roc")
	if roc == nil || len(roc.FPR) == 0 {
		return nil
	}

	n := len(roc.FPR)
	values := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		values = append(values, map[string]any{
			"fpr": roc.FPR[i],
			"tpr": roc.TPR[i],
		})
	}

	// Diagonal reference line data.
	diagonal := []map[string]any{
		{"x": 0.0, "y": 0.0},
		{"x": 1.0, "y": 1.0},
	}

	return map[string]any{
		"$schema":     vegaLiteSchema,
		"title":       fmt.Sprintf("ROC Curve (AUC = %.4f)", roc.AUC),
		"width":       400,
		"height":      400,
		"description": "Receiver Operating Characteristic curve",
		"layer": []any{
			map[string]any{
				"data": map[string]any{"values": diagonal},
				"mark": map[string]any{
					"type":        "line",
					"strokeDash":  []int{4, 4},
					"color":       "#cccccc",
					"strokeWidth": 1,
				},
				"encoding": map[string]any{
					"x": map[string]any{"field": "x", "type": "quantitative"},
					"y": map[string]any{"field": "y", "type": "quantitative"},
				},
			},
			map[string]any{
				"data": map[string]any{"values": values},
				"mark": map[string]any{
					"type":    "line",
					"tooltip": true,
					"color":   "#1f77b4",
				},
				"encoding": map[string]any{
					"x": map[string]any{
						"field": "fpr",
						"type":  "quantitative",
						"title": "False Positive Rate",
						"scale": map[string]any{"domain": []float64{0, 1}},
					},
					"y": map[string]any{
						"field": "tpr",
						"type":  "quantitative",
						"title": "True Positive Rate",
						"scale": map[string]any{"domain": []float64{0, 1}},
					},
				},
			},
		},
	}
}

func confusionMatrixSpec(result *bench.RunResult) map[string]any {
	cm := findCurve[bench.ConfusionMatrixDetail](result, "confusion_matrix")
	if cm == nil || len(cm.Labels) == 0 {
		return nil
	}

	var values []map[string]any
	for i, actual := range cm.Labels {
		for j, predicted := range cm.Labels {
			if i < len(cm.Matrix) && j < len(cm.Matrix[i]) {
				values = append(values, map[string]any{
					"actual":    actual,
					"predicted": predicted,
					"count":     cm.Matrix[i][j],
				})
			}
		}
	}

	return map[string]any{
		"$schema":     vegaLiteSchema,
		"title":       "Confusion Matrix",
		"width":       400,
		"height":      400,
		"description": "Confusion matrix heatmap",
		"data":        map[string]any{"values": values},
		"mark":        map[string]any{"type": "rect", "tooltip": true},
		"encoding": map[string]any{
			"x": map[string]any{
				"field": "predicted",
				"type":  "nominal",
				"title": "Predicted",
			},
			"y": map[string]any{
				"field": "actual",
				"type":  "nominal",
				"title": "Actual",
			},
			"color": map[string]any{
				"field": "count",
				"type":  "quantitative",
				"title": "Count",
				"scale": map[string]any{"scheme": "blues"},
			},
			"text": map[string]any{
				"field": "count",
				"type":  "quantitative",
			},
		},
		"layer": []any{
			map[string]any{
				"mark": map[string]any{"type": "rect", "tooltip": true},
			},
			map[string]any{
				"mark": map[string]any{
					"type":  "text",
					"color": "#333333",
				},
				"encoding": map[string]any{
					"text": map[string]any{
						"field": "count",
						"type":  "quantitative",
					},
				},
			},
		},
	}
}

func thresholdSweepSpec(result *bench.RunResult) map[string]any {
	points := findAllCurves[bench.ThresholdPoint](result)
	if len(points) == 0 {
		return nil
	}

	var values []map[string]any
	for _, p := range points {
		values = append(values,
			map[string]any{"threshold": p.Threshold, "value": p.Precision, "metric": "Precision"},
			map[string]any{"threshold": p.Threshold, "value": p.Recall, "metric": "Recall"},
			map[string]any{"threshold": p.Threshold, "value": p.F1, "metric": "F1"},
			map[string]any{"threshold": p.Threshold, "value": p.Accuracy, "metric": "Accuracy"},
		)
	}

	return map[string]any{
		"$schema":     vegaLiteSchema,
		"title":       "Threshold Sweep",
		"width":       400,
		"height":      300,
		"description": "Classification metrics across decision thresholds",
		"data":        map[string]any{"values": values},
		"mark":        map[string]any{"type": "line", "tooltip": true},
		"encoding": map[string]any{
			"x": map[string]any{
				"field": "threshold",
				"type":  "quantitative",
				"title": "Threshold",
			},
			"y": map[string]any{
				"field": "value",
				"type":  "quantitative",
				"title": "Score",
				"scale": map[string]any{"domain": []float64{0, 1}},
			},
			"color": map[string]any{
				"field": "metric",
				"type":  "nominal",
				"title": "Metric",
			},
		},
	}
}

func calibrationSpec(result *bench.RunResult) map[string]any {
	cal := findCurve[bench.CalibrationCurve](result, "calibration")
	if cal == nil || len(cal.PredictedProbability) == 0 {
		return nil
	}

	n := len(cal.PredictedProbability)
	values := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		if i < len(cal.ActualFrequency) {
			values = append(values, map[string]any{
				"predicted": cal.PredictedProbability[i],
				"actual":    cal.ActualFrequency[i],
			})
		}
	}

	diagonal := []map[string]any{
		{"x": 0.0, "y": 0.0},
		{"x": 1.0, "y": 1.0},
	}

	return map[string]any{
		"$schema":     vegaLiteSchema,
		"title":       "Calibration Curve",
		"width":       400,
		"height":      400,
		"description": "Calibration curve comparing predicted probability to actual frequency",
		"layer": []any{
			map[string]any{
				"data": map[string]any{"values": diagonal},
				"mark": map[string]any{
					"type":        "line",
					"strokeDash":  []int{4, 4},
					"color":       "#cccccc",
					"strokeWidth": 1,
				},
				"encoding": map[string]any{
					"x": map[string]any{"field": "x", "type": "quantitative"},
					"y": map[string]any{"field": "y", "type": "quantitative"},
				},
			},
			map[string]any{
				"data": map[string]any{"values": values},
				"mark": map[string]any{
					"type":    "line",
					"tooltip": true,
					"point":   true,
					"color":   "#ff7f0e",
				},
				"encoding": map[string]any{
					"x": map[string]any{
						"field": "predicted",
						"type":  "quantitative",
						"title": "Mean Predicted Probability",
						"scale": map[string]any{"domain": []float64{0, 1}},
					},
					"y": map[string]any{
						"field": "actual",
						"type":  "quantitative",
						"title": "Actual Frequency",
						"scale": map[string]any{"domain": []float64{0, 1}},
					},
				},
			},
		},
	}
}

func branchComparisonSpec(result *bench.RunResult) map[string]any {
	if len(result.Branches) == 0 {
		return nil
	}

	// Collect all metric names across branches.
	metricSet := make(map[string]struct{})
	names := make([]string, 0, len(result.Branches))
	for name := range result.Branches {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for mk := range result.Branches[name].Metrics {
			metricSet[mk] = struct{}{}
		}
	}
	metricNames := make([]string, 0, len(metricSet))
	for mk := range metricSet {
		metricNames = append(metricNames, mk)
	}
	sort.Strings(metricNames)

	if len(metricNames) == 0 {
		return nil
	}

	var values []map[string]any
	for _, branchName := range names {
		b := result.Branches[branchName]
		for _, mk := range metricNames {
			values = append(values, map[string]any{
				"branch": branchName,
				"metric": mk,
				"value":  b.Metrics[mk],
			})
		}
	}

	return map[string]any{
		"$schema":     vegaLiteSchema,
		"title":       "Branch Comparison",
		"width":       400,
		"height":      300,
		"description": "Grouped bar chart comparing metrics across evaluator branches",
		"data":        map[string]any{"values": values},
		"mark":        map[string]any{"type": "bar", "tooltip": true},
		"encoding": map[string]any{
			"x": map[string]any{
				"field": "metric",
				"type":  "nominal",
				"title": "Metric",
			},
			"y": map[string]any{
				"field": "value",
				"type":  "quantitative",
				"title": "Value",
			},
			"xOffset": map[string]any{
				"field": "branch",
				"type":  "nominal",
			},
			"color": map[string]any{
				"field": "branch",
				"type":  "nominal",
				"title": "Branch",
			},
		},
	}
}

func scoreDistributionSpec(result *bench.RunResult) map[string]any {
	// First try ScoreDistribution curves.
	dists := findAllCurves[bench.ScoreDistribution](result)

	// If no explicit distributions, build from samples.
	if len(dists) == 0 && len(result.Samples) > 0 {
		return scoreDistributionFromSamples(result)
	}
	if len(dists) == 0 {
		return nil
	}

	var values []map[string]any
	for _, d := range dists {
		for i, count := range d.Counts {
			if i < len(d.Bins) {
				values = append(values, map[string]any{
					"bin":   d.Bins[i],
					"count": count,
					"label": d.Label,
				})
			}
		}
	}

	if len(values) == 0 {
		return nil
	}

	return map[string]any{
		"$schema":     vegaLiteSchema,
		"title":       "Score Distribution",
		"width":       400,
		"height":      300,
		"description": "Distribution of prediction scores per label",
		"data":        map[string]any{"values": values},
		"mark":        map[string]any{"type": "bar", "tooltip": true, "opacity": 0.7},
		"encoding": map[string]any{
			"x": map[string]any{
				"field": "bin",
				"type":  "quantitative",
				"title": "Score",
				"bin":   map[string]any{"binned": true},
			},
			"y": map[string]any{
				"field": "count",
				"type":  "quantitative",
				"title": "Count",
				"stack":  nil,
			},
			"color": map[string]any{
				"field": "label",
				"type":  "nominal",
				"title": "Label",
			},
		},
	}
}

func scoreDistributionFromSamples(result *bench.RunResult) map[string]any {
	var values []map[string]any
	for _, s := range result.Samples {
		values = append(values, map[string]any{
			"score": s.Score,
			"label": s.Label,
		})
	}
	if len(values) == 0 {
		return nil
	}

	return map[string]any{
		"$schema":     vegaLiteSchema,
		"title":       "Score Distribution by Label",
		"width":       400,
		"height":      300,
		"description": "Histogram of prediction scores per label",
		"data":        map[string]any{"values": values},
		"mark":        map[string]any{"type": "bar", "tooltip": true, "opacity": 0.7},
		"encoding": map[string]any{
			"x": map[string]any{
				"field": "score",
				"type":  "quantitative",
				"title": "Score",
				"bin":   true,
			},
			"y": map[string]any{
				"aggregate": "count",
				"type":      "quantitative",
				"title":     "Count",
				"stack":     nil,
			},
			"color": map[string]any{
				"field": "label",
				"type":  "nominal",
				"title": "Label",
			},
		},
	}
}
