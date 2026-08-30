package bench_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/bench"
	"github.com/kbukum/gokit/bench/metric"
	"github.com/kbukum/gokit/llm"
	llmtestutil "github.com/kbukum/gokit/llm/testutil"
)

func judgeContextMetric(t *testing.T, provider llm.Provider) bench.RunContextMetric[string] {
	t.Helper()
	m, err := metric.LLMJudge[string](provider, "judge-model", metric.DefaultJudgePrompt())
	if err != nil {
		t.Fatalf("LLMJudge: %v", err)
	}
	return metric.AsRunContextMetric[string](m)
}

// When a judge metric scores a run, its model and prompt version are lifted into
// the run provenance so the scores map to the exact judge that produced them.
func TestRunnerRecordsJudgeProvenance(t *testing.T) {
	t.Parallel()

	loader := writeContextDataset(t)
	provider := llmtestutil.NewFakeProvider(llmtestutil.WithResponder(
		func(context.Context, llm.CompletionRequest) (llm.CompletionResponse, error) {
			return llmtestutil.TextResponse(`{"score": 0.9}`), nil
		}))

	runner := bench.NewBenchRunner(
		bench.WithContextMetrics(judgeContextMetric(t, provider)),
	)
	runner.Register("model", bench.EvaluatorFunc("m", func(_ context.Context, _ []byte) (bench.Prediction[string], error) {
		return bench.Prediction[string]{Label: "positive", Score: 0.9}, nil
	}))

	result, err := runner.Run(context.Background(), loader)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Provenance.Judges) != 1 {
		t.Fatalf("Provenance.Judges = %d entries, want 1", len(result.Provenance.Judges))
	}
	judge := result.Provenance.Judges[0]
	if judge.Model != "judge-model" {
		t.Errorf("Judges[0].Model = %q, want judge-model", judge.Model)
	}
	if judge.PromptVersion != "1.0.0" {
		t.Errorf("Judges[0].PromptVersion = %q, want 1.0.0", judge.PromptVersion)
	}
	if judge.Metric == "" {
		t.Error("Judges[0].Metric is empty, want the judge metric name")
	}
}

// A run may carry several judge metrics with distinct models or prompts; each is
// recorded in provenance, in suite order, rather than collapsing to a single
// identity, so every judge's scores map to the exact judge that produced them.
func TestRunnerRecordsEveryJudgeProvenance(t *testing.T) {
	t.Parallel()

	loader := writeContextDataset(t)
	provider := llmtestutil.NewFakeProvider(llmtestutil.WithResponder(
		func(context.Context, llm.CompletionRequest) (llm.CompletionResponse, error) {
			return llmtestutil.TextResponse(`{"score": 0.9}`), nil
		}))

	first, err := metric.LLMJudge[string](provider, "judge-a", metric.DefaultJudgePrompt())
	if err != nil {
		t.Fatalf("LLMJudge first: %v", err)
	}
	second, err := metric.LLMJudge[string](provider, "judge-b", metric.DefaultJudgePrompt())
	if err != nil {
		t.Fatalf("LLMJudge second: %v", err)
	}
	runner := bench.NewBenchRunner(
		bench.WithContextMetrics(
			metric.AsRunContextMetric[string](first),
			metric.AsRunContextMetric[string](second),
		),
	)
	runner.Register("model", bench.EvaluatorFunc("m", func(_ context.Context, _ []byte) (bench.Prediction[string], error) {
		return bench.Prediction[string]{Label: "positive", Score: 0.9}, nil
	}))

	result, err := runner.Run(context.Background(), loader)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Provenance.Judges) != 2 {
		t.Fatalf("Provenance.Judges = %d entries, want 2", len(result.Provenance.Judges))
	}
	if got := result.Provenance.Judges[0].Model; got != "judge-a" {
		t.Errorf("Judges[0].Model = %q, want judge-a", got)
	}
	if got := result.Provenance.Judges[1].Model; got != "judge-b" {
		t.Errorf("Judges[1].Model = %q, want judge-b", got)
	}
}

// A run with no judge metric omits the judge provenance fields entirely.
func TestRunnerOmitsJudgeProvenanceWhenAbsent(t *testing.T) {
	t.Parallel()

	loader := writeContextDataset(t)
	runner := bench.NewBenchRunner(
		bench.WithMetrics(metric.AsRunMetric[string](mustBinaryClassification[string](t, "positive"))),
	)
	runner.Register("model", bench.EvaluatorFunc("m", func(_ context.Context, _ []byte) (bench.Prediction[string], error) {
		return bench.Prediction[string]{Label: "positive"}, nil
	}))

	result, err := runner.Run(context.Background(), loader)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Provenance.Judges) != 0 {
		t.Errorf("Provenance.Judges = %d entries, want none when no judge metric ran", len(result.Provenance.Judges))
	}
}
