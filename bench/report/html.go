package report

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"sort"
	"strings"

	"github.com/kbukum/gokit/bench"
)

// HTML creates a reporter that outputs a self-contained HTML report.
// The report embeds Vega-Lite specs and loads Vega-Embed from CDN.
func HTML() Reporter {
	return &htmlReporter{}
}

type htmlReporter struct{}

func (r *htmlReporter) Name() string { return "html" }

func (r *htmlReporter) Generate(w io.Writer, result *bench.RunResult) error {
	specs := VegaLiteSpecs(result)

	var b strings.Builder
	b.WriteString(htmlHead(result))
	b.WriteString(htmlSummary(result))
	b.WriteString(htmlMetrics(result))
	b.WriteString(htmlCharts(specs))
	b.WriteString(htmlBranches(result))
	b.WriteString(htmlSamples(result))
	b.WriteString(htmlFooter(specs))

	_, err := io.WriteString(w, b.String())
	return err
}

func htmlHead(result *bench.RunResult) string {
	title := "Bench Report"
	if result.Tag != "" {
		title += " — " + html.EscapeString(result.Tag)
	}
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + title + `</title>
<script src="https://cdn.jsdelivr.net/npm/vega@5"></script>
<script src="https://cdn.jsdelivr.net/npm/vega-lite@5"></script>
<script src="https://cdn.jsdelivr.net/npm/vega-embed@6"></script>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #f5f5f5; color: #333; }
  header { background: #1a1a2e; color: #fff; padding: 1.5rem 2rem; }
  header h1 { font-size: 1.5rem; font-weight: 600; }
  header p { font-size: 0.875rem; opacity: 0.8; margin-top: 0.25rem; }
  .container { max-width: 1200px; margin: 0 auto; padding: 1.5rem; }
  section { margin-bottom: 1.5rem; }
  section h2 { font-size: 1.25rem; margin-bottom: 0.75rem; color: #1a1a2e; border-bottom: 2px solid #e0e0e0; padding-bottom: 0.25rem; }
  .card { background: #fff; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.1); padding: 1.25rem; margin-bottom: 1rem; }
  table { width: 100%; border-collapse: collapse; }
  th, td { text-align: left; padding: 0.5rem 0.75rem; border-bottom: 1px solid #eee; }
  th { font-weight: 600; font-size: 0.8rem; text-transform: uppercase; color: #666; }
  td { font-size: 0.9rem; }
  .chart-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(450px, 1fr)); gap: 1rem; }
  .chart-card { background: #fff; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.1); padding: 1rem; }
  .chart-card h3 { font-size: 0.95rem; margin-bottom: 0.5rem; color: #444; }
  .samples-table { max-height: 400px; overflow-y: auto; }
  .correct { color: #2e7d32; }
  .incorrect { color: #c62828; }
  .badge { display: inline-block; padding: 0.15rem 0.5rem; border-radius: 4px; font-size: 0.75rem; font-weight: 600; }
  .badge-ok { background: #e8f5e9; color: #2e7d32; }
  .badge-err { background: #ffebee; color: #c62828; }
</style>
</head>
<body>
<header>
  <h1>` + title + `</h1>
  <p>Run ` + html.EscapeString(result.ID) + ` &middot; ` + result.Timestamp.Format("2006-01-02 15:04:05") + ` &middot; ` + result.Duration.String() + `</p>
</header>
<div class="container">
`
}

func htmlSummary(result *bench.RunResult) string {
	var b strings.Builder
	b.WriteString(`<section><h2>Summary</h2><div class="card"><table>`)
	b.WriteString(`<tr><th>Dataset</th><td>` + html.EscapeString(result.Dataset.Name) + ` v` + html.EscapeString(result.Dataset.Version) + `</td></tr>`)
	_, _ = fmt.Fprintf(&b, `<tr><th>Samples</th><td>%d</td></tr>`, result.Dataset.SampleCount)

	if len(result.Dataset.LabelDistribution) > 0 {
		labels := make([]string, 0, len(result.Dataset.LabelDistribution))
		for l := range result.Dataset.LabelDistribution {
			labels = append(labels, l)
		}
		sort.Strings(labels)
		parts := make([]string, 0, len(labels))
		for _, l := range labels {
			parts = append(parts, fmt.Sprintf("%s: %d", html.EscapeString(l), result.Dataset.LabelDistribution[l]))
		}
		b.WriteString(`<tr><th>Labels</th><td>` + strings.Join(parts, ", ") + `</td></tr>`)
	}

	correct := 0
	for _, s := range result.Samples {
		if s.Correct {
			correct++
		}
	}
	if len(result.Samples) > 0 {
		pct := float64(correct) / float64(len(result.Samples)) * 100
		_, _ = fmt.Fprintf(&b, `<tr><th>Accuracy</th><td>%d / %d (%.1f%%)</td></tr>`, correct, len(result.Samples), pct)
	}

	b.WriteString(`</table></div></section>`)
	return b.String()
}

func htmlMetrics(result *bench.RunResult) string {
	if len(result.Metrics) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section><h2>Metrics</h2><div class="card"><table>`)
	b.WriteString(`<tr><th>Metric</th><th>Value</th><th>Per-Label</th></tr>`)
	for _, m := range result.Metrics {
		perLabel := "—"
		if len(m.Values) > 0 {
			keys := make([]string, 0, len(m.Values))
			for k := range m.Values {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			var parts []string
			for _, k := range keys {
				parts = append(parts, fmt.Sprintf("%s: %.4f", html.EscapeString(k), m.Values[k]))
			}
			perLabel = strings.Join(parts, ", ")
		}
		_, _ = fmt.Fprintf(&b, `<tr><td>%s</td><td>%.4f</td><td>%s</td></tr>`,
			html.EscapeString(m.Name), m.Value, perLabel)
	}
	b.WriteString(`</table></div></section>`)
	return b.String()
}

func htmlCharts(specs map[string]any) string {
	if len(specs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section><h2>Charts</h2><div class="chart-grid">`)

	// Deterministic order.
	filenames := make([]string, 0, len(specs))
	for fn := range specs {
		filenames = append(filenames, fn)
	}
	sort.Strings(filenames)

	for i, fn := range filenames {
		chartID := fmt.Sprintf("chart-%d", i)
		name := strings.TrimSuffix(fn, ".vl.json")
		name = strings.ReplaceAll(name, "_", " ")
		_, _ = fmt.Fprintf(&b, `<div class="chart-card"><h3>%s</h3><div id="%s"></div>`, html.EscapeString(name), chartID) //nolint:gocritic // HTML attribute quoting, not Go string quoting

		specJSON, _ := json.Marshal(specs[fn])
		_, _ = fmt.Fprintf(&b, `<script type="application/json" id="%s-spec">%s</script>`, chartID, string(specJSON))
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div></section>`)
	return b.String()
}

func htmlBranches(result *bench.RunResult) string {
	if len(result.Branches) == 0 {
		return ""
	}

	names := make([]string, 0, len(result.Branches))
	for name := range result.Branches {
		names = append(names, name)
	}
	sort.Strings(names)

	// Collect all metric keys.
	metricSet := make(map[string]struct{})
	for _, name := range names {
		for mk := range result.Branches[name].Metrics {
			metricSet[mk] = struct{}{}
		}
	}
	metricKeys := make([]string, 0, len(metricSet))
	for mk := range metricSet {
		metricKeys = append(metricKeys, mk)
	}
	sort.Strings(metricKeys)

	var b strings.Builder
	b.WriteString(`<section><h2>Branch Comparison</h2><div class="card"><table>`)
	b.WriteString(`<tr><th>Branch</th><th>Tier</th>`)
	for _, mk := range metricKeys {
		b.WriteString(`<th>` + html.EscapeString(mk) + `</th>`)
	}
	b.WriteString(`<th>Avg+</th><th>Avg−</th><th>Duration</th><th>Errors</th></tr>`)

	for _, name := range names {
		br := result.Branches[name]
		_, _ = fmt.Fprintf(&b, `<tr><td>%s</td><td>%d</td>`,
			html.EscapeString(br.Name), br.Tier)
		for _, mk := range metricKeys {
			_, _ = fmt.Fprintf(&b, `<td>%.4f</td>`, br.Metrics[mk])
		}
		_, _ = fmt.Fprintf(&b, `<td>%.4f</td><td>%.4f</td><td>%s</td><td>%d</td></tr>`,
			br.AvgScorePositive, br.AvgScoreNegative, br.Duration.String(), br.Errors)
	}

	b.WriteString(`</table></div></section>`)
	return b.String()
}

func htmlSamples(result *bench.RunResult) string {
	if len(result.Samples) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section><h2>Sample Details</h2><div class="card samples-table"><table>`)
	b.WriteString(`<tr><th>ID</th><th>Label</th><th>Predicted</th><th>Score</th><th>Correct</th><th>Duration</th><th>Error</th></tr>`)

	for _, s := range result.Samples {
		correctClass := "correct"
		correctBadge := "badge-ok"
		correctText := "✓"
		if !s.Correct {
			correctClass = "incorrect"
			correctBadge = "badge-err"
			correctText = "✗"
		}
		errText := "—"
		if s.Error != "" {
			errText = html.EscapeString(s.Error)
		}
		_, _ = fmt.Fprintf(&b, //nolint:gocritic // HTML attribute quoting, not Go string quoting
			`<tr><td>%s</td><td>%s</td><td>%s</td><td>%.4f</td><td class="%s"><span class="badge %s">%s</span></td><td>%s</td><td>%s</td></tr>`,
			html.EscapeString(s.ID), html.EscapeString(s.Label), html.EscapeString(s.Predicted),
			s.Score, correctClass, correctBadge, correctText, s.Duration.String(), errText)
	}

	b.WriteString(`</table></div></section>`)
	return b.String()
}

func htmlFooter(specs map[string]any) string {
	var b strings.Builder

	// Render each chart with vegaEmbed.
	if len(specs) > 0 {
		filenames := make([]string, 0, len(specs))
		for fn := range specs {
			filenames = append(filenames, fn)
		}
		sort.Strings(filenames)

		b.WriteString("<script>\n")
		b.WriteString("document.addEventListener('DOMContentLoaded', function() {\n")
		for i := range filenames {
			chartID := fmt.Sprintf("chart-%d", i)
			_, _ = fmt.Fprintf(&b,
				"  var spec%d = JSON.parse(document.getElementById('%s-spec').textContent);\n"+
					"  vegaEmbed('#%s', spec%d, {actions: false}).catch(console.error);\n",
				i, chartID, chartID, i)
		}
		b.WriteString("});\n")
		b.WriteString("</script>\n")
	}

	b.WriteString("</div>\n</body>\n</html>\n")
	return b.String()
}
