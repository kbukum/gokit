//go:build !race

package testutil

// RaceEnabled reports whether this executable was built with Go's race detector.
// Child builders can pass it to GoBuild.Race to match the owning test's instrumentation.
const RaceEnabled = false
