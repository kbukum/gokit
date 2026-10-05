package report

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/kbukum/gokit/bench"
)

// Markdown returns a reporter that outputs GitHub-flavored Markdown tables.
func Markdown() Reporter {
	return &markdownReporter{}
}

type markdownReporter struct{}

func (r *markdownReporter) Name() string { return "markdown" }

func (r *markdownReporter) Generate(ctx context.Context, w io.Writer, input Input) error {
	if err := validateInput(ctx, input); err != nil {
		return err
	}
	result := input.Result
	rows, err := preview(ctx, input, maxMarkdownSamples)
	if err != nil {
		return err
	}
	var b strings.Builder

	r.writeSummary(&b, result)
	r.writeMetrics(&b, result)
	r.writeConfusionMatrix(&b, result)
	r.writeBranches(&b, result)
	r.writeSamples(&b, rows)
	if input.Store != nil && result.Dataset.SampleCount > len(rows) {
		fmt.Fprintf(&b, "\n> Showing %d of %d samples.\n", len(rows), result.Dataset.SampleCount)
	}
	if input.Diff != nil {
		b.WriteString("\n" + input.Diff.Summary())
	}

	_, err = io.WriteString(w, b.String())
	return err
}

func (r *markdownReporter) writeSummary(b *strings.Builder, result *bench.RunResult) {
	b.WriteString("# Bench Run Report\n\n")
	b.WriteString("| Field | Value |\n")
	b.WriteString("|-------|-------|\n")
	fmt.Fprintf(b, "| **Run ID** | `%s` |\n", result.ID)
	fmt.Fprintf(b, "| **Timestamp** | %s |\n", result.Timestamp.Format("2006-01-02 15:04:05 UTC"))
	fmt.Fprintf(b, "| **Dataset** | %s (v%s) |\n", result.Dataset.Name, result.Dataset.Version)
	fmt.Fprintf(b, "| **Samples** | %d |\n", result.Dataset.SampleCount)
	if result.Tag != "" {
		fmt.Fprintf(b, "| **Tag** | %s |\n", result.Tag)
	}
	fmt.Fprintf(b, "| **Duration** | %s |\n", result.Duration)
	b.WriteString("\n")
}

func (r *markdownReporter) writeMetrics(b *strings.Builder, result *bench.RunResult) {
	if len(result.Metrics) == 0 {
		return
	}
	b.WriteString("## Metrics\n\n")
	b.WriteString("| Metric | Value | Detail |\n")
	b.WriteString("|--------|------:|--------|\n")
	for metricIndex := range result.Metrics {
		m := &result.Metrics[metricIndex]
		detail := ""
		if m.Values != nil {
			parts := formatValues(m.Values)
			detail = strings.Join(parts, ", ")
		}
		fmt.Fprintf(b, "| %s | %.4f | %s |\n", m.Name, m.Value, detail)
	}
	b.WriteString("\n")
}

func (r *markdownReporter) writeConfusionMatrix(b *strings.Builder, result *bench.RunResult) {
	for metricIndex := range result.Metrics {
		m := &result.Metrics[metricIndex]
		cm := m.Confusion
		if cm == nil {
			continue
		}
		b.WriteString("## Confusion Matrix\n\n")
		b.WriteString("| |")
		for _, label := range cm.Labels {
			fmt.Fprintf(b, " **%s** |", label)
		}
		b.WriteString("\n|---|")
		for range cm.Labels {
			b.WriteString("---:|")
		}
		b.WriteString("\n")
		for i, row := range cm.Matrix {
			fmt.Fprintf(b, "| **%s** |", cm.Labels[i])
			for _, val := range row {
				fmt.Fprintf(b, " %d |", val)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		return // Only render the first confusion matrix found.
	}
}

func (r *markdownReporter) writeBranches(b *strings.Builder, result *bench.RunResult) {
	if len(result.Branches) == 0 {
		return
	}
	b.WriteString("## Branches\n\n")
	b.WriteString("| Branch | Tier | Avg+ | Avg− | Errors | Duration |\n")
	b.WriteString("|--------|-----:|-----:|-----:|-------:|---------|\n")

	names := make([]string, 0, len(result.Branches))
	for name := range result.Branches {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		br := result.Branches[name]
		fmt.Fprintf(b, "| %s | %d | %.4f | %.4f | %d | %s |\n",
			br.Name, br.Tier, br.AvgScorePositive, br.AvgScoreNegative, br.Errors, br.Duration)
	}
	b.WriteString("\n")
}

const maxMarkdownSamples = 50

func (r *markdownReporter) writeSamples(b *strings.Builder, samples []bench.SampleResult) {
	if len(samples) == 0 {
		return
	}
	b.WriteString("## Samples\n\n")

	count := len(samples)
	truncated := false
	if count > maxMarkdownSamples {
		truncated = true
		count = maxMarkdownSamples
	}

	b.WriteString("| ID | Label | Predicted | Score | Correct | Duration |\n")
	b.WriteString("|----|-------|-----------|------:|:-------:|---------|\n")
	for _, s := range samples[:count] {
		icon := "✅"
		if !s.Correct {
			icon = "❌"
		}
		if s.Error != "" {
			icon = "⚠️"
		}
		fmt.Fprintf(b, "| %s | %s | %s | %.4f | %s | %s |\n",
			s.ID, s.Label, s.Predicted, s.Score, icon, s.Duration)
	}
	if truncated {
		fmt.Fprintf(b, "\n> Showing %d of %d samples.\n", count, len(samples))
	}
	b.WriteString("\n")
}

// formatValues sorts and formats a map of per-label metric values.
func formatValues(vals map[string]float64) []string {
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%.4f", k, vals[k]))
	}
	return parts
}
