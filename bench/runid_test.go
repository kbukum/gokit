package bench

import (
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/util"
)

// newRunID builds a runner with an injected clock and suffix and returns its generated run ID.
func newRunID(t *testing.T, tag string) string {
	t.Helper()
	clock := util.NewFakeClock(time.Date(2026, 8, 23, 4, 30, 54, 0, time.UTC))
	opts := []RunOption[string]{
		WithClock[string](clock),
		WithIDSuffix[string](func() string { return "fixedsfx" }),
	}
	if tag != "" {
		opts = append(opts, WithTag[string](tag))
	}
	return NewBenchRunner(opts...).generateID()
}

// TestRunIDUntaggedFormat locks the untagged run-id shape run_<ts>_<uuid8>.
func TestRunIDUntaggedFormat(t *testing.T) {
	t.Parallel()
	if got, want := newRunID(t, ""), "run_20260823-043054_fixedsfx"; got != want {
		t.Errorf("untagged run ID = %q, want %q", got, want)
	}
}

// TestRunIDTaggedFormat locks the tagged run-id shape <tag>_<ts>_<uuid8> (D4): a tag replaces the
// "run" prefix but the timestamp and unique suffix still follow, so two tagged runs in the same
// second never collide on one storage file.
func TestRunIDTaggedFormat(t *testing.T) {
	t.Parallel()
	if got, want := newRunID(t, "nightly"), "nightly_20260823-043054_fixedsfx"; got != want {
		t.Errorf("tagged run ID = %q, want %q", got, want)
	}
}

// TestRunIDRandomSuffixIsEightHex locks that the production suffix is an 8-char id fragment, so an
// untagged (or same-tag same-second) run gets a unique, filename-safe identifier.
func TestRunIDRandomSuffixIsEightHex(t *testing.T) {
	t.Parallel()
	suffix := randomIDSuffix()
	if len(suffix) != 8 {
		t.Fatalf("randomIDSuffix() = %q (len %d), want length 8", suffix, len(suffix))
	}
	if strings.ContainsAny(suffix, `/\`) {
		t.Errorf("randomIDSuffix() = %q contains a path separator", suffix)
	}
}
