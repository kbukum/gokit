package bench

import (
	"fmt"
	"math"
)

type BoundKind string

const (
	Min BoundKind = "min"
	Max BoundKind = "max"
)

// Bound selects a metric headline or named subvalue and an explicit bound.
type Bound struct {
	Metric string
	Value  string
	Kind   BoundKind
	Limit  float64
}
type BoundCheck struct {
	Bound  Bound
	Passed bool
	Actual float64
	Reason string
}

// CheckBounds fails closed on missing values and invalid bounds.
func CheckBounds(result *RunResult, bounds []Bound) []BoundCheck {
	checks := make([]BoundCheck, 0, len(bounds))
	for _, bound := range bounds {
		check := BoundCheck{Bound: bound, Reason: fmt.Sprintf("%s.%s: metric or value missing", bound.Metric, bound.Value)}
		if result != nil {
			for metricIndex := range result.Metrics {
				m := &result.Metrics[metricIndex]
				if m.Name != bound.Metric {
					continue
				}
				value, ok := m.Value, true
				if bound.Value != "" {
					value, ok = m.Values[bound.Value]
				}
				if !ok {
					break
				}
				check.Actual = value
				if math.IsNaN(value) || math.IsInf(value, 0) || math.IsNaN(bound.Limit) || math.IsInf(bound.Limit, 0) {
					check.Reason = "non-finite bound or value"
					break
				}
				switch bound.Kind {
				case Min:
					check.Passed = value >= bound.Limit
				case Max:
					check.Passed = value <= bound.Limit
				default:
					check.Reason = "unknown bound kind"
				}
				if check.Passed {
					check.Reason = ""
				} else {
					check.Reason = fmt.Sprintf("%s.%s: %g violates %s %g", bound.Metric, bound.Value, value, bound.Kind, bound.Limit)
				}
				break
			}
		}
		checks = append(checks, check)
	}
	return checks
}
