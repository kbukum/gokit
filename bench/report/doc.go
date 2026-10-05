// Package report renders benchmark summaries and optional stored-record previews.
//
// Every Reporter implements Generate(ctx, writer, Input{Result, Store, Diff}). Store is optional; Markdown and HTML read at most 50 preview records, and Table reads at most 20. JSON preserves the canonical SchemaVersion 2.0 result. All formats render eligibility and verdict when Diff is supplied.
//
// JUnit accepts WithBounds with explicit bench.Min or bench.Max limits. Missing metrics or subvalues fail closed. A supplied comparison adds an eligibility test case and a verdict property.
package report
