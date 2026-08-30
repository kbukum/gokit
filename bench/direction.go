package bench

import (
	"encoding/json"
	"fmt"
	"net/http"

	apperrors "github.com/kbukum/gokit/errors"
)

// Direction is the optimization direction of a metric: whether higher or lower
// values are better, or whether the metric is descriptive with no preferred
// direction.
//
// Run comparison uses this to classify a metric change as an improvement or a
// regression. Without it, every increase would be treated as an improvement —
// wrong for error metrics (lower is better) and meaningless for descriptive
// metrics such as token usage.
//
// The zero value is [HigherIsBetter], so the many accuracy-style metrics for
// which higher is better are classified correctly by [RunComparator] without
// setting it explicitly.
//
// The JSON encoding is the snake_case string of each value
// (higher_is_better/lower_is_better/neutral), shared byte-for-byte with the
// sibling rskit bench schema so the two kits emit an interchangeable contract.
type Direction int

const (
	// HigherIsBetter marks a metric where larger values are better (accuracy,
	// F1, AUC). The default.
	HigherIsBetter Direction = iota
	// LowerIsBetter marks a metric where smaller values are better (error
	// rates, loss, calibration error).
	LowerIsBetter
	// Neutral marks a descriptive metric with no optimization direction (token
	// usage, counts). It is never an improvement or a regression.
	Neutral
)

// directionString maps each direction to its stable JSON spelling.
var directionString = map[Direction]string{
	HigherIsBetter: "higher_is_better",
	LowerIsBetter:  "lower_is_better",
	Neutral:        "neutral",
}

// String returns the stable snake_case name of the direction.
func (d Direction) String() string {
	if s, ok := directionString[d]; ok {
		return s
	}
	return fmt.Sprintf("Direction(%d)", int(d))
}

// IsImprovement classifies a signed value delta (new - old) as an improvement
// in this direction. A [Neutral] metric is never an improvement.
func (d Direction) IsImprovement(delta float64) bool {
	switch d {
	case HigherIsBetter:
		return delta > 0
	case LowerIsBetter:
		return delta < 0
	default:
		return false
	}
}

// IsRegression classifies a signed value delta (new - old) as a regression in
// this direction. A [Neutral] metric never regresses.
func (d Direction) IsRegression(delta float64) bool {
	switch d {
	case HigherIsBetter:
		return delta < 0
	case LowerIsBetter:
		return delta > 0
	default:
		return false
	}
}

// MarshalJSON encodes the direction as its snake_case string.
func (d Direction) MarshalJSON() ([]byte, error) {
	s, ok := directionString[d]
	if !ok {
		return nil, apperrors.New(apperrors.ErrCodeInvalidInput,
			fmt.Sprintf("bench: invalid metric direction %d", int(d)),
			http.StatusBadRequest)
	}
	return []byte(`"` + s + `"`), nil
}

// UnmarshalJSON decodes a direction from its snake_case string. The JSON string
// is decoded with [encoding/json] first, so a valid escaped spelling (for
// example "\u0068igher_is_better") is accepted the same as its plain form,
// matching the sibling serde contract. An unknown or malformed value is a typed
// error rather than a silent default.
func (d *Direction) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return apperrors.New(apperrors.ErrCodeInvalidInput,
			fmt.Sprintf("bench: metric direction must be a JSON string, got %s", data),
			http.StatusBadRequest)
	}
	for dir, name := range directionString {
		if name == s {
			*d = dir
			return nil
		}
	}
	return apperrors.New(apperrors.ErrCodeInvalidInput,
		fmt.Sprintf("bench: unknown metric direction %q", s),
		http.StatusBadRequest)
}
