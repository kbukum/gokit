package metric

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/bench"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/resilience"
)

const (
	// judgeBaseName is the metric name stem; the judge model and prompt identity
	// are appended to form the full, comparison-safe name.
	judgeBaseName = "llm_judge"
	// defaultJudgeThreshold is the score at or above which a graded pair counts as
	// a pass for the reported pass rate.
	defaultJudgeThreshold = 0.5
	// defaultJudgeTimeout is the per-call judging timeout on the default resilience
	// policy; the baseline requires a timeout on every remote call.
	defaultJudgeTimeout = 30 * time.Second
	// defaultJudgeConcurrency bounds in-flight judge calls to a small, principled
	// fan-out rather than one goroutine per sample, so a large run applies
	// backpressure to the judge instead of flooding it.
	defaultJudgeConcurrency = 4
	// defaultJudgeMaxTokens caps tokens the judge may generate per call. The
	// verdict is a small {"score": …, "rationale": …} object, so a conservative
	// bound keeps an untrusted or misbehaving judge from generating an unbounded,
	// costly reply; the resilience timeout bounds elapsed time, not response size.
	defaultJudgeMaxTokens = 256
	// defaultJudgeMaxPromptBytes bounds the rendered prompt (reference +
	// prediction filled into the template) sent to the judge per sample. The
	// labels are untrusted, so without a cap a single oversized label could
	// allocate and send an arbitrarily large request and incur unbounded
	// input-token cost despite the output-token, timeout, and concurrency limits;
	// an over-long rendered prompt is rejected before the provider is called.
	defaultJudgeMaxPromptBytes = 128 * 1024
)

// LLMJudge scores each prediction against its reference by asking an injected [llm.Provider] to grade the pair, using a versioned [JudgePrompt] so a run records exactly which prompt produced its scores. The primary [Result.Value] and avg_score in [Result.Values] report the mean judge score; pass_rate reports the fraction of samples scored at or above the threshold. The judge model and prompt identity are recorded in [Result.Detail] and lifted into the run's [bench.RunProvenance]; the threshold is folded into the metric name (not published as a value) so two runs with different thresholds carry different names and their threshold-dependent pass rates are never compared as if equivalent.
//
// The reply is treated as untrusted: the judge is asked for a JSON object, and the reply is parsed into a typed [JudgeVerdict] with shape and range validation. A malformed or non-JSON reply, an out-of-range or missing score, a completion that did not finish normally (truncated by the token cap, stopped by a content filter, ended by a provider error), or an over-long reply surfaces as a typed external-service [apperrors.AppError] — never a fabricated success-shaped score and never a panic. This parser is the trust boundary on reply shape, not a prompt-injection detector: a syntactically valid {"score": 1} still parses, so the injection defense is the data-only framing in the prompt's system instruction, not this parser.
//
// Every provider call runs through the canonical [resilience.Policy] (default: a per-call 30-second timeout) so a slow or hung judge cannot stall a run, requests a bounded max_tokens, and honors cancellation; calls are issued with bounded concurrency across samples (default 4), never an unbounded goroutine fan-out. The first sample failure cancels a derived context and stops scheduling further samples, so one malformed reply or provider error does not bill every remaining call. Scores are reduced in input order so identical verdicts aggregate to identical bits regardless of completion order. Empty input yields a zeroed result without calling the provider.
//
// A provider may resolve the requested model to a different backend model or route samples across models; when a resolved model id is reported it is tracked, a run whose samples resolve to more than one model is rejected as incomparable, and a single resolved model that differs from the requested one is recorded in provenance so scores are never silently published under the wrong identity.
//
// The metric name embeds the model and prompt identity, including a fingerprint of the rubric (template body + system instruction) — for example llm_judge[openai/gpt-4o-mini@gokit.builtin.judge@1.0.0#a1b2c3d4e5f6:t0.5] — so runs scored by a different judge model, prompt, rubric, or threshold stay distinct in provenance and are never joined as compatible by name alone; editing the rubric without bumping the version still changes the fingerprint and thus the identity.
//
// LLMJudge returns an error if provider is nil (including a typed-nil interface value), if model is empty or blank (a judge run must record a reproducible requested model rather than fall through to a provider default), if prompt was not built through [ParseJudgePrompt]/[DefaultJudgePrompt] (a zero-value prompt binding neither placeholder), if the threshold is not a finite value in [0, 1], if concurrency is not positive, if the configured resilience policy carries no positive timeout, or if that policy configures retries (a judge call is not idempotent, so a retried call would bill a duplicate completion and could replace the verdict nondeterministically).
func LLMJudge[L comparable](provider llm.Provider, model string, prompt JudgePrompt, opts ...JudgeOption[L]) (ContextMetric[L], error) {
	if isNilLLMProvider(provider) {
		return nil, apperrors.New(apperrors.ErrCodeInvalidInput,
			"llm_judge: LLMJudge requires a non-nil llm.Provider", http.StatusBadRequest)
	}
	if !prompt.bound() {
		return nil, apperrors.New(apperrors.ErrCodeInvalidInput,
			"llm_judge: prompt must be built with ParseJudgePrompt or DefaultJudgePrompt; it must bind both {reference} and {prediction}",
			http.StatusBadRequest)
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, apperrors.New(apperrors.ErrCodeInvalidInput,
			"llm_judge: model must not be empty; a judge run must record a reproducible requested model rather than fall through to a provider default",
			http.StatusBadRequest)
	}
	m := &llmJudge[L]{
		provider:       provider,
		model:          model,
		prompt:         prompt,
		threshold:      defaultJudgeThreshold,
		concurrency:    defaultJudgeConcurrency,
		maxTokens:      defaultJudgeMaxTokens,
		maxPromptBytes: defaultJudgeMaxPromptBytes,
		policy:         resilience.NewPolicy().WithTimeout(defaultJudgeTimeout),
		extract:        func(l L) string { return fmt.Sprintf("%v", l) },
	}
	for _, o := range opts {
		o(m)
	}
	if err := validateJudgeThreshold(m.threshold); err != nil {
		return nil, err
	}
	if m.concurrency < 1 {
		return nil, apperrors.New(apperrors.ErrCodeInvalidInput,
			fmt.Sprintf("llm_judge: concurrency %d must be greater than zero", m.concurrency),
			http.StatusBadRequest)
	}
	if m.policy == nil || m.policy.Timeout <= 0 {
		return nil, apperrors.New(apperrors.ErrCodeInvalidInput,
			"llm_judge: resilience policy must carry a positive timeout so every judge call is time-bounded",
			http.StatusBadRequest)
	}
	if m.policy.Retry != nil {
		return nil, apperrors.New(apperrors.ErrCodeInvalidInput,
			"llm_judge: resilience policy must not configure retries; a judge call is not idempotent, so a retry would bill a duplicate completion and could replace the verdict nondeterministically",
			http.StatusBadRequest)
	}
	m.name = judgeMetricName(provider.Name(), model, prompt, m.threshold)
	return m, nil
}

// validateJudgeThreshold rejects a threshold that is not a finite score in [0, 1]. A non-finite threshold would be compared against every verdict and break the pass-rate, so it is caught at construction as a typed invalid-input error.
func validateJudgeThreshold(threshold float64) error {
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 1 {
		return apperrors.New(apperrors.ErrCodeInvalidInput,
			fmt.Sprintf("llm_judge: threshold %v must be a finite value within [0, 1]", threshold),
			http.StatusBadRequest)
	}
	return nil
}

// judgeMetricName builds the collision-safe metric identity: provider, model, prompt id/version, a rubric fingerprint, and the pass threshold, each escaped so distinct component tuples cannot alias. The fingerprint hashes the template body and system instruction, so a rubric edited without a version bump still yields a distinct name. The threshold is part of the identity because the pass rate is only comparable across runs that used the same cutoff.
func judgeMetricName(provider, model string, prompt JudgePrompt, threshold float64) string {
	id := escapeIdentity(model)
	if provider != "" {
		id = escapeIdentity(provider) + "/" + id
	}
	return fmt.Sprintf("%s[%s@%s@%s#%s:t%s]", judgeBaseName, id,
		escapeIdentity(prompt.id), escapeIdentity(prompt.version),
		prompt.fingerprint()[:12], formatThreshold(threshold))
}

type llmJudge[L comparable] struct {
	provider       llm.Provider
	model          string
	prompt         JudgePrompt
	name           string
	threshold      float64
	concurrency    int
	maxTokens      int
	maxPromptBytes int
	policy         *resilience.Policy
	extract        func(L) string
}

func (m *llmJudge[L]) Name() string { return m.name }

func (m *llmJudge[L]) Compute(ctx context.Context, scored []bench.ScoredSample[L]) (Result, error) {
	if len(scored) == 0 {
		return m.zeroed(), nil
	}

	// A derived cancellable context lets the first failure stop the run: the outer
	// loop stops scheduling once the context is done and in-flight calls abort, so
	// one bad reply or provider error does not bill every remaining judge call.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	scores := make([]float64, len(scored))
	resolved := make([]string, len(scored))
	errs := make([]error, len(scored))
	// Cap the semaphore at the sample count so a large configured concurrency does
	// not over-allocate the buffer for a small run (and cannot be driven toward an
	// out-of-memory allocation when concurrency is huge relative to the work).
	sem := make(chan struct{}, min(m.concurrency, len(scored)))
	var wg sync.WaitGroup
	for i := range scored {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := ctx.Err(); err != nil {
				errs[idx] = judgeProviderError(err)
				return
			}
			score, model, err := m.grade(ctx, scored[idx])
			scores[idx], resolved[idx] = score, model
			if err != nil {
				errs[idx] = withSampleIndex(idx, scored[idx].Sample.ID, err)
				cancel()
			}
		}(i)
	}
	wg.Wait()

	if err := selectJudgeError(errs); err != nil {
		return Result{}, err
	}

	// If the loop stopped scheduling because the caller canceled (or timed out)
	// the context, some samples were never scored; surface that rather than
	// reducing over unscored zeros. A first-failure cancellation is already
	// returned above as its genuine error, so this only fires for external
	// cancellation.
	if err := ctx.Err(); err != nil {
		return Result{}, judgeProviderError(err)
	}

	resolvedModel, err := resolveJudgeModel(resolved)
	if err != nil {
		return Result{}, err
	}

	var sum float64
	matches := 0
	for _, s := range scores {
		sum += s
		if s >= m.threshold {
			matches++
		}
	}

	n := float64(len(scored))
	avg := sum / n
	return Result{
		Name:  m.name,
		Value: avg,
		Values: map[string]float64{
			"avg_score": avg,
			"pass_rate": float64(matches) / n,
		},
		Detail: m.detail(resolvedModel),
	}, nil
}

// withSampleIndex tags a per-sample failure with the input index and sample id
// of the sample that produced it, so [selectJudgeError] returns an error that
// names the failing sample rather than an anonymous provider error. It preserves
// the typed cause: when err is an [apperrors.AppError] the identifiers are added
// to its details (Code and status are untouched); otherwise the identifiers are
// carried on a wrapping error that still unwraps to the original cause.
func withSampleIndex(idx int, sampleID string, err error) error {
	if appErr, ok := apperrors.AsAppError(err); ok {
		return appErr.WithDetail("sample_index", idx).WithDetail("sample_id", sampleID)
	}
	return fmt.Errorf("judge sample %d (%q): %w", idx, sampleID, err)
}

// selectJudgeError returns a single deterministic failure from the per-sample
// errors, or nil when all succeeded. A genuine failure (anything other than the
// cancellation this metric propagates on the first error) is preferred at its
// lowest index, so the returned error names the sample that actually failed
// (its index and id are carried in the error's details) rather than a sibling
// that was merely canceled as a consequence; only when every error is a
// cancellation (for example the caller canceled the run) is the lowest-index
// cancellation returned.
func selectJudgeError(errs []error) error {
	var canceled error
	for _, err := range errs {
		if err == nil {
			continue
		}
		if isCanceledError(err) {
			if canceled == nil {
				canceled = err
			}
			continue
		}
		return err
	}
	return canceled
}

// isCanceledError reports whether err is the canceled error this metric raises
// when it aborts remaining work after a failure (or when the caller cancels).
func isCanceledError(err error) bool {
	if appErr, ok := apperrors.AsAppError(err); ok {
		return appErr.Code == apperrors.ErrCodeCanceled
	}
	return false
}

// resolveJudgeModel collapses the per-sample resolved model ids into the single
// backend model that produced the run's scores, or "" when the provider reported
// none for any sample. Resolution is all-or-nothing: either every sample reports
// the same non-empty resolved model, or none do. A run whose samples resolve to
// more than one model — or that mixes samples with a reported model and samples
// without — is rejected as incomparable, since a partial or mixed resolution
// means the run's scores were produced by more than one judge under a single
// requested-model identity.
func resolveJudgeModel(resolved []string) (string, error) {
	model := ""
	sawEmpty := false
	for _, rm := range resolved {
		if rm == "" {
			sawEmpty = true
			continue
		}
		if model == "" {
			model = rm
			continue
		}
		if rm != model {
			return "", invalidJudgeReply(fmt.Sprintf(
				"judge resolved samples to multiple models (%q and %q); their scores are not comparable", model, rm))
		}
	}
	if model != "" && sawEmpty {
		return "", invalidJudgeReply(fmt.Sprintf(
			"judge resolved some samples to model %q and left others unreported; their scores are not comparable", model))
	}
	return model, nil
}

// grade renders the prompt for one sample, calls the provider through the resilience policy, and parses the untrusted reply into a validated score. It also returns the provider-resolved model id (trimmed, possibly empty) so the caller can detect model routing.
func (m *llmJudge[L]) grade(ctx context.Context, s bench.ScoredSample[L]) (score float64, resolvedModel string, err error) {
	prediction := m.extract(s.Prediction.Label)
	reference := m.extract(s.Sample.Label)
	rendered, err := m.prompt.render(prediction, reference)
	if err != nil {
		return 0, "", apperrors.New(apperrors.ErrCodeInternal,
			"llm_judge: failed to render prompt template", http.StatusInternalServerError).WithCause(err)
	}
	if len(rendered) > m.maxPromptBytes {
		return 0, "", apperrors.New(apperrors.ErrCodeInvalidInput,
			fmt.Sprintf("llm_judge: rendered prompt of %d bytes exceeds the %d-byte bound; the reference/prediction labels are too large to judge",
				len(rendered), m.maxPromptBytes),
			http.StatusBadRequest)
	}

	temperature := 0.0
	req := llm.CompletionRequest{
		Model:        m.model,
		SystemPrompt: m.prompt.system,
		Messages:     []chat.Message{chat.User(rendered)},
		Temperature:  &temperature,
		MaxTokens:    m.maxTokens,
	}

	resp, err := resilience.Execute(ctx, m.policy, func(callCtx context.Context) (llm.CompletionResponse, error) {
		return m.provider.Execute(callCtx, req)
	})
	if err != nil {
		return 0, "", judgeProviderError(err)
	}

	resolvedModel = strings.TrimSpace(resp.Model)

	if reasonErr := ensureCompleteReason(resp.StopReason); reasonErr != nil {
		return 0, resolvedModel, reasonErr
	}

	reply := resp.Text()
	if len(reply) > maxJudgeReplyBytes {
		return 0, resolvedModel, invalidJudgeReply(fmt.Sprintf(
			"judge reply of %d bytes exceeds the %d-byte bound", len(reply), maxJudgeReplyBytes))
	}

	verdict, err := parseJudgeVerdict(reply)
	if err != nil {
		return 0, resolvedModel, err
	}
	return verdict.Score, resolvedModel, nil
}

func (m *llmJudge[L]) zeroed() Result {
	return Result{
		Name:   m.name,
		Value:  0,
		Values: map[string]float64{"avg_score": 0, "pass_rate": 0},
		Detail: m.detail(""),
	}
}

// detail records the judge model and prompt identity so a persisted result carries its scoring provenance and the runner can lift it into [bench.RunProvenance]. When the provider resolved the request to a single backend model that differs from the requested one, that resolved id is recorded too, so provenance reflects the model that actually produced the scores.
func (m *llmJudge[L]) detail(resolvedModel string) map[string]any {
	d := map[string]any{
		bench.DetailJudgeProvider:          m.provider.Name(),
		bench.DetailJudgeModel:             m.model,
		bench.DetailJudgePromptID:          m.prompt.id,
		bench.DetailJudgePromptVersion:     m.prompt.version,
		bench.DetailJudgePromptFingerprint: m.prompt.fingerprint(),
	}
	if resolvedModel != "" && resolvedModel != m.model {
		d[bench.DetailJudgeResolvedModel] = resolvedModel
	}
	return d
}

// isNilLLMProvider reports whether provider is nil, including a typed-nil interface value (a nil *T stored in the interface) that a plain provider == nil check misses and that would otherwise panic when Compute dispatches through it.
func isNilLLMProvider(provider llm.Provider) bool {
	if provider == nil {
		return true
	}
	v := reflect.ValueOf(provider)
	switch v.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}
