package report

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/kbukum/gokit/bench"
)

// CSV returns a reporter that outputs flat tabular CSV with one row per metric.
func CSV() Reporter {
	return &csvReporter{}
}

type csvReporter struct{}

func (r *csvReporter) Name() string { return "csv" }

func (r *csvReporter) Generate(w io.Writer, result *bench.RunResult) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// Header row.
	if err := cw.Write([]string{"metric_name", "value", "details"}); err != nil {
		return err
	}

	for _, m := range result.Metrics {
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
