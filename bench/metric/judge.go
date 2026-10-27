package metric

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/bench"
	apperrors "github.com/kbukum/gokit/errors"
)

const (
	judgeBaseName           = "llm_judge"
	defaultJudgeThreshold   = 0.5
	defaultJudgeConcurrency = 4
)

// LLMJudge scores each prediction with a [Judge] in score mode. The sample input is the task input, the prediction is the response, and the sample label is the reference when the rubric uses one. Value and avg_score are the mean normalized score over judged items; pass_rate is the share of judged items at or above the threshold; judged and no_verdict count items. Items with no verdict are left out of the mean and counted, never scored as zero. When no item gets a verdict the metric fails, since there is nothing to report.
//
// Calls run with bounded concurrency, and the first provider failure cancels the rest. The judge's identity is recorded in [Result.Judge].
func LLMJudge[L comparable](judge *Judge, opts ...LLMJudgeOption[L]) (ContextMetric[L], error) {
	if judge == nil {
		return nil, apperrors.InvalidInput("llm_judge", "LLMJudge requires a judge")
	}
	m := &llmJudge[L]{judge: judge, threshold: defaultJudgeThreshold, concurrency: defaultJudgeConcurrency, extract: func(l L) string { return fmt.Sprintf("%v", l) }}
	for _, o := range opts {
		o(m)
	}
	if math.IsNaN(m.threshold) || m.threshold < 0 || m.threshold > 1 {
		return nil, apperrors.InvalidInput("llm_judge", fmt.Sprintf("threshold %v must be within [0, 1]", m.threshold))
	}
	if m.concurrency < 1 {
		return nil, apperrors.InvalidInput("llm_judge", fmt.Sprintf("concurrency %d must be greater than zero", m.concurrency))
	}
	p := judge.Provenance(JudgeScorePromptID)
	id := escapeIdentity(p.Model)
	if p.Provider != "" {
		id = escapeIdentity(p.Provider) + "/" + id
	}
	m.name = fmt.Sprintf("%s[%s@%s@%s#%s:t%s]", judgeBaseName, id, escapeIdentity(p.PromptID), escapeIdentity(p.PromptVersion),
		p.PromptFingerprint[:12], formatThreshold(m.threshold))
	return m, nil
}

type llmJudge[L comparable] struct {
	judge       *Judge
	name        string
	threshold   float64
	concurrency int
	extract     func(L) string
}

func (m *llmJudge[L]) Name() string { return m.name }

func (m *llmJudge[L]) Compute(ctx context.Context, scored []bench.ScoredSample[L]) (Result, error) {
	if len(scored) == 0 {
		return Result{}, apperrors.InvalidInput("llm_judge", "there are no samples to judge")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	verdicts := make([]Verdict, len(scored))
	errs := make([]error, len(scored))
	sem := make(chan struct{}, min(m.concurrency, len(scored)))
	var wg sync.WaitGroup
	for i := range scored {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			in := JudgeInput{Input: []ai.ContentPart{ai.Text{Text: strings.ToValidUTF8(string(scored[i].Sample.Input), "\uFFFD")}}}
			if m.judge.rubric.UseReference {
				in.Reference = m.extract(scored[i].Sample.Label)
			}
			v, err := m.judge.Score(ctx, in, m.extract(scored[i].Prediction.Label))
			verdicts[i] = v
			if err != nil {
				errs[i] = withSample(i, scored[i].Sample.ID, err)
				cancel()
			}
		}(i)
	}
	wg.Wait()
	if err := firstFailure(errs); err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, judgeProviderError(err)
	}
	model, err := resolvedModel(verdicts)
	if err != nil {
		return Result{}, err
	}
	var sum float64
	judged, passed := 0, 0
	for i := range verdicts {
		v := &verdicts[i]
		if v.Kind != VerdictJudged {
			continue
		}
		judged++
		sum += v.Score
		if v.Score >= m.threshold {
			passed++
		}
	}
	if judged == 0 {
		return Result{}, apperrors.New(apperrors.ErrCodeExternalService, fmt.Sprintf("llm_judge: none of %d samples got a verdict", len(scored)))
	}
	avg := sum / float64(judged)
	detail := m.judge.Provenance(JudgeScorePromptID)
	detail.Metric = m.name
	if model != "" && model != detail.Model {
		detail.ResolvedModel = model
	}
	return Result{
		Name: m.name, Value: avg,
		Values: map[string]float64{"avg_score": avg, "pass_rate": float64(passed) / float64(judged), "judged": float64(judged), "no_verdict": float64(len(scored) - judged)},
		Judge:  &detail,
	}, nil
}

func withSample(idx int, id string, err error) error {
	if appErr, ok := apperrors.AsAppError(err); ok {
		return appErr.WithDetail("sample_index", idx).WithDetail("sample_id", id)
	}
	return fmt.Errorf("judge sample %d (%q): %w", idx, id, err)
}

// firstFailure prefers the lowest-index genuine failure over a cancellation it caused.
func firstFailure(errs []error) error {
	var canceled error
	for _, err := range errs {
		if err == nil {
			continue
		}
		if appErr, ok := apperrors.AsAppError(err); ok && appErr.Code == apperrors.ErrCodeCanceled {
			if canceled == nil {
				canceled = err
			}
			continue
		}
		return err
	}
	return canceled
}

// resolvedModel returns the one backend model every verdict reported, or "" when none did. Mixed or partial resolution is refused: those scores came from more than one judge.
func resolvedModel(verdicts []Verdict) (string, error) {
	model, empty := "", false
	for i := range verdicts {
		v := &verdicts[i]
		switch {
		case v.Model == "":
			empty = true
		case model == "":
			model = v.Model
		case v.Model != model:
			return "", apperrors.New(apperrors.ErrCodeExternalService, fmt.Sprintf("llm_judge: judge resolved samples to %q and %q; their scores are not comparable", model, v.Model))
		}
	}
	if model != "" && empty {
		return "", apperrors.New(apperrors.ErrCodeExternalService, fmt.Sprintf("llm_judge: judge resolved some samples to %q and left others unreported", model))
	}
	return model, nil
}
