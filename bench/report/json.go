package report

import (
	"encoding/json"
	"io"
	"sort"

	"github.com/kbukum/gokit/bench"
)

// JSON returns a reporter that outputs the canonical Bench JSON format.
func JSON() Reporter {
	return &jsonReporter{}
}

type jsonReporter struct{}

func (r *jsonReporter) Name() string { return "json" }

func (r *jsonReporter) Generate(w io.Writer, result *bench.RunResult) error {
	report := r.buildReport(result)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(report)
}

// jsonReport mirrors the desired JSON output format with $schema and version at top level.
type jsonReport struct {
	Schema  string         `json:"$schema"`
	Version string         `json:"version"`
	Run     jsonRun        `json:"run"`
	Dataset jsonDataset    `json:"dataset"`
	Metrics []jsonMetric   `json:"metrics"`
	Curves  map[string]any `json:"curves,omitempty"`
	Branch  []jsonBranch   `json:"branches,omitempty"`
	Samples []jsonSample   `json:"samples,omitempty"`
}

type jsonRun struct {
	ID         string `json:"id"`
	Timestamp  string `json:"timestamp"`
	Tag        string `json:"tag,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

type jsonDataset struct {
	Name              string         `json:"name"`
	Version           string         `json:"version"`
	SampleCount       int            `json:"sample_count"`
	LabelDistribution map[string]int `json:"label_distribution"`
}

type jsonMetric struct {
	Name   string             `json:"name"`
	Value  float64            `json:"value"`
	Values map[string]float64 `json:"values,omitempty"`
	Detail any                `json:"detail,omitempty"`
}

type jsonBranch struct {
	Name             string             `json:"name"`
	Tier             int                `json:"tier"`
	Metrics          map[string]float64 `json:"metrics"`
	AvgScorePositive float64            `json:"avg_score_positive"`
	AvgScoreNegative float64            `json:"avg_score_negative"`
	DurationMs       int64              `json:"duration_ms"`
	Errors           int                `json:"errors"`
}

type jsonSample struct {
	ID           string             `json:"id"`
	Label        string             `json:"label"`
	Predicted    string             `json:"predicted"`
	Score        float64            `json:"score"`
	Correct      bool               `json:"correct"`
	BranchScores map[string]float64 `json:"branch_scores,omitempty"`
	DurationMs   int64              `json:"duration_ms"`
	Error        string             `json:"error,omitempty"`
}

func (r *jsonReporter) buildReport(result *bench.RunResult) jsonReport {
	metrics := make([]jsonMetric, len(result.Metrics))
	for i, m := range result.Metrics {
		metrics[i] = jsonMetric{
			Name:   m.Name,
			Value:  m.Value,
			Values: m.Values,
			Detail: m.Detail,
		}
	}

	// Sort branches by name for deterministic output.
	var branches []jsonBranch
	if len(result.Branches) > 0 {
		names := make([]string, 0, len(result.Branches))
		for name := range result.Branches {
			names = append(names, name)
		}
		sort.Strings(names)
		branches = make([]jsonBranch, 0, len(names))
		for _, name := range names {
			b := result.Branches[name]
			branches = append(branches, jsonBranch{
				Name:             b.Name,
				Tier:             b.Tier,
				Metrics:          b.Metrics,
				AvgScorePositive: b.AvgScorePositive,
				AvgScoreNegative: b.AvgScoreNegative,
				DurationMs:       b.Duration.Milliseconds(),
				Errors:           b.Errors,
			})
		}
	}

	samples := make([]jsonSample, len(result.Samples))
	for i, s := range result.Samples {
		samples[i] = jsonSample{
			ID:           s.ID,
			Label:        s.Label,
			Predicted:    s.Predicted,
			Score:        s.Score,
			Correct:      s.Correct,
			BranchScores: s.BranchScores,
			DurationMs:   s.Duration.Milliseconds(),
			Error:        s.Error,
		}
	}

	return jsonReport{
		Schema:  bench.SchemaURL,
		Version: bench.SchemaVersion,
		Run: jsonRun{
			ID:         result.ID,
			Timestamp:  result.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			Tag:        result.Tag,
			DurationMs: result.Duration.Milliseconds(),
		},
		Dataset: jsonDataset{
			Name:              result.Dataset.Name,
			Version:           result.Dataset.Version,
			SampleCount:       result.Dataset.SampleCount,
			LabelDistribution: result.Dataset.LabelDistribution,
		},
		Metrics: metrics,
		Curves:  result.Curves,
		Branch:  branches,
		Samples: samples,
	}
}
