// Package report generates formatted output from bench evaluation results.
//
// Every reporter implements the [Reporter] interface:
//
//	type Reporter interface {
//	    Name() string
//	    Generate(w io.Writer, result *bench.RunResult) error
//	}
//
// # Available Formats
//
//   - [JSON] — canonical Bench JSON with $schema and version fields
//   - [Markdown] — GitHub-flavored Markdown tables
//   - [Table] — ASCII table for terminal display
//   - [CSV] — comma-separated values for spreadsheet import
//   - [JUnit] — JUnit XML for CI integration
//   - [VegaLite] — Vega-Lite JSON specifications for interactive charts
//   - [HTML] — self-contained HTML report with embedded charts
//
// # Usage
//
//	r := report.JSON()
//	var buf bytes.Buffer
//	if err := r.Generate(&buf, result); err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(buf.String())
//
// Multiple reporters can be used on the same result:
//
//	reporters := []report.Reporter{
//	    report.JSON(),
//	    report.Markdown(),
//	    report.Table(),
//	}
//	for _, r := range reporters {
//	    f, _ := os.Create("report." + r.Name())
//	    r.Generate(f, result)
//	    f.Close()
//	}
package report
