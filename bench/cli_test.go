package bench

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewCLIRunnerDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	cli := NewCLIRunner(storage)

	// Default output should not be nil.
	if cli.out == nil {
		t.Fatal("default out is nil")
	}
}

func TestNewCLIRunnerWithOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	var buf bytes.Buffer
	cli := NewCLIRunner(storage, WithOutput(&buf))

	if cli.out != &buf {
		t.Error("WithOutput did not set output writer")
	}
}

func TestCLIRunnerListRunsEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	var buf bytes.Buffer
	cli := NewCLIRunner(storage, WithOutput(&buf))

	err := cli.ListRuns(context.Background())
	if err != nil {
		t.Fatalf("ListRuns() error: %v", err)
	}

	if !strings.Contains(buf.String(), "No runs found") {
		t.Errorf("output = %q, want 'No runs found'", buf.String())
	}
}

func TestCLIRunnerListRuns(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	r1 := makeTestResult("run-alpha", "v1", "my-dataset")
	r1.Timestamp = time.Now().Add(-1 * time.Hour)
	storage.Save(ctx, r1)

	r2 := makeTestResult("run-beta", "v2", "my-dataset")
	r2.Timestamp = time.Now()
	storage.Save(ctx, r2)

	var buf bytes.Buffer
	cli := NewCLIRunner(storage, WithOutput(&buf))

	err := cli.ListRuns(ctx)
	if err != nil {
		t.Fatalf("ListRuns() error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "run-alpha") {
		t.Errorf("output missing run-alpha: %q", output)
	}
	if !strings.Contains(output, "run-beta") {
		t.Errorf("output missing run-beta: %q", output)
	}
	if !strings.Contains(output, "my-dataset") {
		t.Errorf("output missing dataset name: %q", output)
	}
}

func TestCLIRunnerListRunsWithTag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	r := makeTestResult("run-tagged", "special", "ds")
	storage.Save(ctx, r)

	var buf bytes.Buffer
	cli := NewCLIRunner(storage, WithOutput(&buf))

	err := cli.ListRuns(ctx)
	if err != nil {
		t.Fatalf("ListRuns() error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "[special]") {
		t.Errorf("output missing tag [special]: %q", output)
	}
}

func TestCLIRunnerShowRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	r := makeTestResult("run-show", "v1", "test-dataset")
	r.Dataset.Version = "2.0"
	r.Branches = map[string]BranchResult{
		"main": {Name: "main", Tier: 1, Duration: 50 * time.Millisecond, Errors: 0},
	}
	storage.Save(ctx, r)

	var buf bytes.Buffer
	cli := NewCLIRunner(storage, WithOutput(&buf))

	err := cli.ShowRun(ctx, "run-show")
	if err != nil {
		t.Fatalf("ShowRun() error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "run-show") {
		t.Errorf("output missing run ID: %q", output)
	}
	if !strings.Contains(output, "test-dataset") {
		t.Errorf("output missing dataset name: %q", output)
	}
	if !strings.Contains(output, "v2.0") {
		t.Errorf("output missing version: %q", output)
	}
}

func TestCLIRunnerShowRunNotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	var buf bytes.Buffer
	cli := NewCLIRunner(storage, WithOutput(&buf))

	err := cli.ShowRun(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
}

func TestCLIRunnerCompareRuns(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	base := makeTestResult("run-base", "", "ds")
	base.Metrics = []MetricResult{
		{Name: "f1", Value: 0.80, Values: map[string]float64{"f1": 0.80}},
	}
	base.Samples = []SampleResult{
		{ID: "s1", Correct: true},
		{ID: "s2", Correct: false},
	}
	storage.Save(ctx, base)

	target := makeTestResult("run-target", "", "ds")
	target.Metrics = []MetricResult{
		{Name: "f1", Value: 0.90, Values: map[string]float64{"f1": 0.90}},
	}
	target.Samples = []SampleResult{
		{ID: "s1", Correct: true},
		{ID: "s2", Correct: true},
	}
	storage.Save(ctx, target)

	var buf bytes.Buffer
	cli := NewCLIRunner(storage, WithOutput(&buf))

	err := cli.CompareRuns(ctx, "run-base", "run-target")
	if err != nil {
		t.Fatalf("CompareRuns() error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "run-base") {
		t.Errorf("output missing base ID: %q", output)
	}
	if !strings.Contains(output, "run-target") {
		t.Errorf("output missing target ID: %q", output)
	}
	if !strings.Contains(output, "Comparing") {
		t.Errorf("output missing 'Comparing': %q", output)
	}
}

func TestCLIRunnerCompareRunsNotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	var buf bytes.Buffer
	cli := NewCLIRunner(storage, WithOutput(&buf))

	err := cli.CompareRuns(context.Background(), "a", "b")
	if err == nil {
		t.Fatal("expected error when base run not found")
	}
}

func TestCLIRunnerCompareLatest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	r1 := makeTestResult("run-older", "", "ds")
	r1.Timestamp = time.Now().Add(-1 * time.Hour)
	r1.Metrics = []MetricResult{{Name: "f1", Value: 0.70, Values: map[string]float64{"f1": 0.70}}}
	storage.Save(ctx, r1)

	r2 := makeTestResult("run-newer", "", "ds")
	r2.Timestamp = time.Now()
	r2.Metrics = []MetricResult{{Name: "f1", Value: 0.80, Values: map[string]float64{"f1": 0.80}}}
	storage.Save(ctx, r2)

	var buf bytes.Buffer
	cli := NewCLIRunner(storage, WithOutput(&buf))

	err := cli.CompareLatest(ctx)
	if err != nil {
		t.Fatalf("CompareLatest() error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Comparing") {
		t.Errorf("output missing 'Comparing': %q", output)
	}
}

func TestCLIRunnerCompareLatestNotEnoughRuns(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	r1 := makeTestResult("run-only", "", "ds")
	storage.Save(ctx, r1)

	var buf bytes.Buffer
	cli := NewCLIRunner(storage, WithOutput(&buf))

	err := cli.CompareLatest(ctx)
	if err == nil {
		t.Fatal("expected error with only 1 run")
	}
	if !strings.Contains(err.Error(), "need at least 2 runs") {
		t.Errorf("error = %q, want mention of 'need at least 2 runs'", err.Error())
	}
}

func TestCLIRunnerShowRunWithMetrics(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	r := makeTestResult("run-metrics", "", "ds")
	r.Metrics = []MetricResult{
		{
			Name:  "classification",
			Value: 0.85,
			Values: map[string]float64{
				"precision": 0.90,
				"recall":    0.80,
			},
		},
	}
	storage.Save(ctx, r)

	var buf bytes.Buffer
	cli := NewCLIRunner(storage, WithOutput(&buf))

	err := cli.ShowRun(ctx, "run-metrics")
	if err != nil {
		t.Fatalf("ShowRun() error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Metrics:") {
		t.Errorf("output missing 'Metrics:': %q", output)
	}
	if !strings.Contains(output, "classification") {
		t.Errorf("output missing 'classification': %q", output)
	}
}
