package metric

import (
	"time"

	"github.com/kbukum/gokit/resilience"
)

// JudgeOption configures a [Judge].
type JudgeOption func(*Judge)

// WithJudgeTimeout sets the per-call timeout. A non-positive duration keeps the default of 30 seconds.
func WithJudgeTimeout(d time.Duration) JudgeOption {
	return func(j *Judge) {
		if d > 0 {
			j.policy = j.policy.WithTimeout(d)
		}
	}
}

// WithJudgePolicy routes every judge call through p. It must carry a positive timeout and no retries; [NewJudge] refuses anything else. A nil policy is ignored.
func WithJudgePolicy(p *resilience.Policy) JudgeOption {
	return func(j *Judge) {
		if p != nil {
			j.policy = p
		}
	}
}

// WithJudgeMaxTokens caps the tokens the judge may generate per call; the default is 256. A cap that truncates the verdict gives no verdict. Values below 1 keep the default.
func WithJudgeMaxTokens(n int) JudgeOption {
	return func(j *Judge) {
		if n >= 1 {
			j.maxTokens = n
		}
	}
}

// WithJudgeMaxPromptBytes caps the bytes of input, reference, outputs and media data sent per call; the default is 128 KiB. A larger item is refused before the provider is called. Values below 1 keep the default.
func WithJudgeMaxPromptBytes(n int) JudgeOption {
	return func(j *Judge) {
		if n >= 1 {
			j.maxPromptBytes = n
		}
	}
}

// LLMJudgeOption configures the [LLMJudge] metric.
type LLMJudgeOption[L comparable] func(*llmJudge[L])

// WithJudgeThreshold sets the normalized score at or above which an item counts toward pass_rate; the default is 0.5. It is part of the metric identity.
func WithJudgeThreshold[L comparable](threshold float64) LLMJudgeOption[L] {
	return func(m *llmJudge[L]) { m.threshold = threshold }
}

// WithJudgeConcurrency bounds concurrent judge calls; the default is 4. Values below 1 are refused.
func WithJudgeConcurrency[L comparable](n int) LLMJudgeOption[L] {
	return func(m *llmJudge[L]) { m.concurrency = n }
}

// WithJudgeExtractor sets how a label becomes text for the judge; the default uses fmt %v. A nil extractor is ignored.
func WithJudgeExtractor[L comparable](fn func(L) string) LLMJudgeOption[L] {
	return func(m *llmJudge[L]) {
		if fn != nil {
			m.extract = fn
		}
	}
}
