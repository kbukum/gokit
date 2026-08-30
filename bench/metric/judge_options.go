package metric

import (
	"time"

	"github.com/kbukum/gokit/resilience"
)

// JudgeOption configures an [LLMJudge] metric.
type JudgeOption[L comparable] func(*llmJudge[L])

// WithJudgeThreshold sets the score at or above which a graded pair counts toward pass_rate. The default is 0.5. The threshold is part of the metric identity, so changing it renames the metric.
func WithJudgeThreshold[L comparable](threshold float64) JudgeOption[L] {
	return func(m *llmJudge[L]) { m.threshold = threshold }
}

// WithJudgeTimeout sets the per-call judging timeout on the resilience policy, leaving any other configured primitives (circuit breaker, …) intact. A non-positive duration is ignored, preserving the bounded default so a stalled judge can never hang a run. To replace the whole policy (for example to add a circuit breaker), pass [WithJudgePolicy]; it must itself carry a positive timeout.
func WithJudgeTimeout[L comparable](d time.Duration) JudgeOption[L] {
	return func(m *llmJudge[L]) {
		if d > 0 {
			m.policy = m.policy.WithTimeout(d)
		}
	}
}

// WithJudgePolicy routes every judge provider call through the given canonical [resilience.Policy] instead of the default, so timeouts and circuit breaking share one configurable seam. The policy must carry a positive timeout and must not configure retries: [LLMJudge] rejects both at construction, because the baseline requires every remote call to be time-bounded and a judge call is not idempotent, so a retry would bill a duplicate completion and could replace the verdict nondeterministically. The default is a per-call 30-second timeout with no retries. A nil policy is ignored.
func WithJudgePolicy[L comparable](p *resilience.Policy) JudgeOption[L] {
	return func(m *llmJudge[L]) {
		if p != nil {
			m.policy = p
		}
	}
}

// WithJudgeMaxPromptBytes caps the byte length of the rendered prompt (the reference and prediction filled into the template) sent to the judge per sample. The labels are untrusted, so the default bounds the request a single oversized label can produce and the input-token cost it incurs; a sample whose rendered prompt exceeds the cap is rejected as invalid input before the provider is called. Values < 1 keep the default.
func WithJudgeMaxPromptBytes[L comparable](n int) JudgeOption[L] {
	return func(m *llmJudge[L]) {
		if n >= 1 {
			m.maxPromptBytes = n
		}
	}
}

// WithJudgeConcurrency sets the maximum number of judge calls issued concurrently. Values < 1 are rejected at construction rather than silently coerced, so a misconfigured fan-out fails loudly.
func WithJudgeConcurrency[L comparable](n int) JudgeOption[L] {
	return func(m *llmJudge[L]) { m.concurrency = n }
}

// WithJudgeMaxTokens caps the tokens the judge may generate per call. The verdict schema is small, so the default keeps an untrusted judge from generating an unbounded reply; raise it only for a verbose rationale. A cap so small it truncates the JSON verdict surfaces as a typed parse error rather than a fabricated score. Values < 1 keep the default.
func WithJudgeMaxTokens[L comparable](n int) JudgeOption[L] {
	return func(m *llmJudge[L]) {
		if n >= 1 {
			m.maxTokens = n
		}
	}
}

// WithJudgeExtractor sets how a label is rendered to the text placed in the judge prompt. The default renders with fmt %v. A nil extractor is ignored.
func WithJudgeExtractor[L comparable](fn func(L) string) JudgeOption[L] {
	return func(m *llmJudge[L]) {
		if fn != nil {
			m.extract = fn
		}
	}
}
