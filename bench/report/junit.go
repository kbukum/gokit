package report

import (
	"encoding/xml"
	"fmt"
	"io"

	"github.com/kbukum/gokit/bench"
)

// JUnitOption configures the JUnit reporter.
type JUnitOption func(*junitReporter)

// WithTargets sets metric targets. Each metric with a matching entry
// becomes a test case that passes if the metric value >= the target.
func WithTargets(targets map[string]float64) JUnitOption {
	return func(r *junitReporter) {
		r.targets = targets
	}
}

// JUnit returns a reporter that outputs JUnit XML for CI/CD integration.
// Metrics with configured targets become test cases: pass if value >= target.
func JUnit(opts ...JUnitOption) Reporter {
	r := &junitReporter{
		targets: make(map[string]float64),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

type junitReporter struct {
	targets map[string]float64
}

func (r *junitReporter) Name() string { return "junit" }

func (r *junitReporter) Generate(w io.Writer, result *bench.RunResult) error {
	suite := r.buildSuite(result)

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

func (r *junitReporter) buildSuite(result *bench.RunResult) junitTestSuites {
	var cases []junitTestCase
	failures := 0

	for _, m := range result.Metrics {
		target, hasTarget := r.targets[m.Name]
		if !hasTarget {
			continue
		}

		tc := junitTestCase{
			Name:      m.Name,
			ClassName: "bench.metrics",
			Time:      "0",
		}

		if m.Value < target {
			failures++
			tc.Failure = &junitFailure{
				Message: fmt.Sprintf("%s: %.4f < %.4f (target)", m.Name, m.Value, target),
				Type:    "BelowTarget",
				Body:    fmt.Sprintf("Metric %q scored %.6f, below target %.6f", m.Name, m.Value, target),
			}
		}
		cases = append(cases, tc)
	}

	props := []junitProperty{
		{Name: "run_id", Value: result.ID},
		{Name: "dataset", Value: result.Dataset.Name},
		{Name: "dataset_version", Value: result.Dataset.Version},
		{Name: "timestamp", Value: result.Timestamp.Format("2006-01-02T15:04:05Z07:00")},
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
