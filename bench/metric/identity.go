package metric

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
)

// identityEscaper escapes every character that structures a metric name or model
// identity — the tuple separators (\ / @), the name brackets ([ ]), and the judge
// name's rubric (#) and threshold (:) delimiters — so a component value containing
// one of them cannot forge or collide with another identity. Extend it in lockstep
// with any new delimiter introduced into a metric name.
var identityEscaper = strings.NewReplacer(
	`\`, `\\`,
	`/`, `\/`,
	`@`, `\@`,
	`[`, `\[`,
	`]`, `\]`,
	`#`, `\#`,
	`:`, `\:`,
)

// escapeIdentity escapes the separator characters that structure a model identity
// and the metric name, so a value containing them cannot forge or collide with
// another identity.
func escapeIdentity(s string) string { return identityEscaper.Replace(s) }

// formatThreshold renders a threshold for a metric name using the shortest
// decimal that round-trips back to the same float64 (strconv 'g', precision -1),
// so 0.5 renders as "0.5" and 0.8 as "0.8" while genuinely distinct cutoffs such
// as 0.50001 and 0.50002 render distinctly rather than colliding under a fixed
// precision. Metric names fold their threshold in through this one helper so the
// same value always renders identically across metrics. Negative zero is folded
// to positive zero first so the semantically equal cutoffs 0 and -0 share one
// identity ("t0") instead of splitting into "t0" and "t-0".
func formatThreshold(threshold float64) string {
	if threshold == 0 {
		// Both -0 and +0 satisfy this comparison; assigning the positive-zero
		// literal rewrites a -0 bit pattern so it formats as "0", not "-0".
		threshold = 0
	}
	return strconv.FormatFloat(threshold, 'g', -1, 64)
}

// validateThresholdRange rejects a threshold that is not a finite value within
// [lo, hi]. A threshold-dependent metric folds its configured cutoff into the
// identity name and detail, so a non-finite or out-of-range value is caught at
// construction as a typed invalid-input error rather than forging a malformed
// name or breaking JSON persistence and run comparison. metric names the
// reporting metric so the message is actionable.
func validateThresholdRange(metric string, threshold, lo, hi float64) error {
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < lo || threshold > hi {
		return apperrors.New(apperrors.ErrCodeInvalidInput,
			fmt.Sprintf("%s: threshold %v must be a finite value within [%v, %v]", metric, threshold, lo, hi),
			http.StatusBadRequest)
	}
	return nil
}
