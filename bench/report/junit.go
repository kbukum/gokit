package report

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"

	"github.com/kbukum/gokit/bench"
)

// JUnitOption configures the JUnit reporter.
type JUnitOption func(*junitReporter)

// WithTargets sets metric targets.
// Each metric with a matching entry becomes a test case that passes if the metric value >= the target.
func WithBounds(bounds ...bench.Bound) JUnitOption {
	return func(r *junitReporter) {
		r.bounds = bounds
	}
}

// JUnit returns a reporter that outputs JUnit XML for CI/CD integration.
// Metrics with configured targets become test cases: pass if value >= target.
func JUnit(opts ...JUnitOption) Reporter {
	r := &junitReporter{}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

type junitReporter struct {
	bounds []bench.Bound
}

func (r *junitReporter) Name() string { return "junit" }

func (r *junitReporter) Generate(ctx context.Context, w io.Writer, input Input) error {
	if err := validateInput(ctx, input); err != nil {
		return err
	}
	suite := r.buildSuite(input)

	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(suite)
}

// XML structures for JUnit output.

type junitTestSuites struct {
	XMLName xml.Name         `xml:"testsuites"`
	Suites  []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	Name       string          `xml:"name,attr"`
	Tests      int             `xml:"tests,attr"`
	Failures   int             `xml:"failures,attr"`
	Time       string          `xml:"time,attr"`
	Properties []junitProperty `xml:"properties>property,omitempty"`
	TestCases  []junitTestCase `xml:"testcase"`
}

type junitProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

func (r *junitReporter) buildSuite(input Input) junitTestSuites {
	result := input.Result
	var cases []junitTestCase
	failures := 0

	for _, check := range bench.CheckBounds(result, r.bounds) {
		tc := junitTestCase{
			Name:      check.Bound.Metric,
			ClassName: "bench.metrics",
			Time:      "0",
		}

		if !check.Passed {
			failures++
			tc.Failure = &junitFailure{
				Message: check.Reason,
				Type:    "BoundViolation",
				Body:    check.Reason,
			}
		}
		cases = append(cases, tc)
	}
	if input.Diff != nil {
		tc := junitTestCase{Name: "eligibility", ClassName: "bench.comparison", Time: "0"}
		if !input.Diff.Eligibility.Eligible {
			failures++
			tc.Failure = &junitFailure{Message: input.Diff.Summary(), Type: "Ineligible"}
		}
		cases = append(cases, tc)
	}

	props := []junitProperty{
		{Name: "run_id", Value: result.ID},
		{Name: "dataset", Value: result.Dataset.Name},
		{Name: "dataset_version", Value: result.Dataset.Version},
		{Name: "timestamp", Value: result.Timestamp.Format("2006-01-02T15:04:05Z07:00")},
	}
	if input.Diff != nil {
		props = append(props, junitProperty{Name: "verdict", Value: input.Diff.Verdict()}, junitProperty{Name: "eligibility", Value: fmt.Sprint(input.Diff.Eligibility.Eligible)})
	}
	if result.Tag != "" {
		props = append(props, junitProperty{Name: "tag", Value: result.Tag})
	}

	suite := junitTestSuite{
		Name:       "bench",
		Tests:      len(cases),
		Failures:   failures,
		Time:       fmt.Sprintf("%.3f", result.Duration.Seconds()),
		Properties: props,
		TestCases:  cases,
	}

	return junitTestSuites{
		Suites: []junitTestSuite{suite},
	}
}
