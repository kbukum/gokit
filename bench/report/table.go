package report

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/kbukum/gokit/bench"
)

// Table returns a reporter that outputs ASCII tables for terminal display.
func Table() Reporter {
	return &tableReporter{}
}

type tableReporter struct{}

func (r *tableReporter) Name() string { return "table" }

func (r *tableReporter) Generate(w io.Writer, result *bench.RunResult) error {
	var b strings.Builder

	r.writeHeader(&b, result)
	r.writeMetrics(&b, result)
	r.writeBranches(&b, result)
	r.writeSamples(&b, result)

	_, err := io.WriteString(w, b.String())
	return err
}

func (r *tableReporter) writeHeader(b *strings.Builder, result *bench.RunResult) {
	b.WriteString("╔══════════════════════════════════════════════════╗\n")
	b.WriteString("║              BENCH RUN REPORT                   ║\n")
	b.WriteString("╚══════════════════════════════════════════════════╝\n")
	fmt.Fprintf(b, "  Run ID:    %s\n", result.ID)
	fmt.Fprintf(b, "  Timestamp: %s\n", result.Timestamp.Format("2006-01-02 15:04:05 UTC"))
	fmt.Fprintf(b, "  Dataset:   %s (v%s, %d samples)\n",
		result.Dataset.Name, result.Dataset.Version, result.Dataset.SampleCount)
	if result.Tag != "" {
		fmt.Fprintf(b, "  Tag:       %s\n", result.Tag)
	}
	fmt.Fprintf(b, "  Duration:  %s\n", result.Duration)
	b.WriteString("\n")
}

func (r *tableReporter) writeMetrics(b *strings.Builder, result *bench.RunResult) {
	if len(result.Metrics) == 0 {
		return
	}

	// Compute column widths.
	nameW := len("Metric")
	for _, m := range result.Metrics {
		if len(m.Name) > nameW {
			nameW = len(m.Name)
		}
	}
	valW := 10 // "Value" + padding

	sep := "+-" + strings.Repeat("-", nameW) + "-+-" + strings.Repeat("-", valW) + "-+-" + strings.Repeat("-", 30) + "-+\n"

	b.WriteString("  METRICS\n")
	b.WriteString("  " + sep)
	fmt.Fprintf(b, "  | %-*s | %*s | %-30s |\n", nameW, "Metric", valW, "Value", "Details")
	b.WriteString("  " + sep)
	for _, m := range result.Metrics {
		detail := ""
		if m.Values != nil {
			parts := formatValues(m.Values)
			detail = strings.Join(parts, ", ")
		}
		if len(detail) > 30 {
			detail = detail[:27] + "..."
		}
		fmt.Fprintf(b, "  | %-*s | %*.4f | %-30s |\n", nameW, m.Name, valW, m.Value, detail)
	}
	b.WriteString("  " + sep)
	b.WriteString("\n")
}

func (r *tableReporter) writeBranches(b *strings.Builder, result *bench.RunResult) {
	if len(result.Branches) == 0 {
		return
	}

	names := make([]string, 0, len(result.Branches))
	for name := range result.Branches {
		names = append(names, name)
	}
	sort.Strings(names)

	nameW := len("Branch")
	for _, name := range names {
		if len(name) > nameW {
			nameW = len(name)
		}
	}

	sep := "+-" + strings.Repeat("-", nameW) + "-+------+--------+--------+--------+\n"

	b.WriteString("  BRANCHES\n")
	b.WriteString("  " + sep)
	fmt.Fprintf(b, "  | %-*s | Tier | Avg+   | Avg-   | Errors |\n", nameW, "Branch")
	b.WriteString("  " + sep)
	for _, name := range names {
		br := result.Branches[name]
		fmt.Fprintf(b, "  | %-*s | %4d | %6.4f | %6.4f | %6d |\n",
			nameW, br.Name, br.Tier, br.AvgScorePositive, br.AvgScoreNegative, br.Errors)
	}
	b.WriteString("  " + sep)
	b.WriteString("\n")
}

const maxTableSamples = 20

func (r *tableReporter) writeSamples(b *strings.Builder, result *bench.RunResult) {
	if len(result.Samples) == 0 {
		return
	}

	count := len(result.Samples)
	truncated := false
	if count > maxTableSamples {
		truncated = true
		count = maxTableSamples
	}

	// Compute ID column width.
	idW := len("ID")
	labelW := len("Label")
	predW := len("Predicted")
	for _, s := range result.Samples[:count] {
		if len(s.ID) > idW {
			idW = len(s.ID)
		}
		if len(s.Label) > labelW {
			labelW = len(s.Label)
		}
		if len(s.Predicted) > predW {
			predW = len(s.Predicted)
		}
	}

	sep := "+-" + strings.Repeat("-", idW) + "-+-" + strings.Repeat("-", labelW) +
		"-+-" + strings.Repeat("-", predW) + "-+--------+----+\n"

	b.WriteString("  SAMPLES\n")
	b.WriteString("  " + sep)
	fmt.Fprintf(b, "  | %-*s | %-*s | %-*s | Score  |    |\n", idW, "ID", labelW, "Label", predW, "Predicted")
	b.WriteString("  " + sep)
	for _, s := range result.Samples[:count] {
		icon := "✅"
		if !s.Correct {
			icon = "❌"
		}
		if s.Error != "" {
			icon = "⚠️"
		}
		fmt.Fprintf(b, "  | %-*s | %-*s | %-*s | %6.4f | %s |\n",
			idW, s.ID, labelW, s.Label, predW, s.Predicted, s.Score, icon)
	}
	b.WriteString("  " + sep)
	if truncated {
		fmt.Fprintf(b, "  ... showing %d of %d samples\n", count, len(result.Samples))
	}
	b.WriteString("\n")
}
