package report

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
)

// CSV returns a reporter that outputs flat tabular CSV with one row per metric.
func CSV() Reporter {
	return &csvReporter{}
}

type csvReporter struct{}

func (r *csvReporter) Name() string { return "csv" }

func (r *csvReporter) Generate(ctx context.Context, w io.Writer, input Input) error {
	if err := validateInput(ctx, input); err != nil {
		return err
	}
	result := input.Result
	cw := csv.NewWriter(w)

	// Header row.
	if err := cw.Write([]string{"metric_name", "value", "details"}); err != nil {
		return err
	}

	for metricIndex := range result.Metrics {
		m := &result.Metrics[metricIndex]
		detail := ""
		if m.Values != nil {
			parts := formatCSVValues(m.Values)
			detail = strings.Join(parts, "; ")
		}
		record := []string{
			m.Name,
			fmt.Sprintf("%.6f", m.Value),
			detail,
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	if input.Diff != nil {
		for _, row := range [][]string{{"eligibility", fmt.Sprint(input.Diff.Eligibility.Eligible), fmt.Sprint(input.Diff.Eligibility.Reasons)}, {"verdict", input.Diff.Verdict(), ""}} {
			if err := cw.Write(row); err != nil {
				return err
			}
		}
	}
	cw.Flush()
	return cw.Error()
}

func formatCSVValues(vals map[string]float64) []string {
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%.6f", k, vals[k]))
	}
	return parts
}
