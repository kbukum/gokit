// Package testutil provides deterministic test doubles for the gokit/bench
// package so tests across the evaluation layer share one hardened set of fakes
// instead of hand-rolling per-test doubles.
//
// [FixedProvenanceProbe] implements bench.ProvenanceProbe with fixed, injected
// values and performs no environment, process, or network access, so a run's
// provenance is byte-identical across executions under a fixed clock and seed.
//
// Example:
//
//	probe := testutil.NewFixedProvenanceProbe(
//		testutil.WithGitCommit("feedface"),
//		testutil.WithHost("ci-runner"),
//		testutil.WithOS("linux"),
//		testutil.WithArch("arm64"),
//	)
//	runner := bench.NewBenchRunner(bench.WithProvenanceProbe[string](probe))
package testutil
