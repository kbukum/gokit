// Package viz generates SVG visualizations from bench evaluation results.
//
// The package produces self-contained SVG images using only the Go standard
// library — no external rendering dependencies are required. Generated
// visualizations include ROC curves, calibration plots, confusion matrix
// heatmaps, score distribution histograms, and branch comparison charts.
//
// # Usage
//
// Pass a [bench.RunResult] to [RenderAll] to generate every applicable chart:
//
//	svgs := viz.RenderAll(result)
//	for name, content := range svgs {
//	    os.WriteFile(name, []byte(content), 0o644)
//	}
//
// Use [RenderOption] functions to customise output:
//
//	svgs := viz.RenderAll(result, viz.WithSize(800, 600))
//
// Individual renderers ([RenderROC], [RenderCalibration], [RenderConfusion],
// [RenderDistribution], [RenderComparison]) are also exported for selective
// generation.
package viz
