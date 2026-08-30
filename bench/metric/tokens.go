package metric

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/kbukum/gokit/bench"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/llm"
)

// tokenStatsBaseName is the metric name stem; the counter identity is appended.
const tokenStatsBaseName = "token_stats"

// TokenStats reports predicted and reference token usage using an injected
// [llm.TokenCounter], so the tokenization strategy (the heuristic core default or
// a real llm/tokenizer contrib adapter) is chosen by injection rather than wired
// in here.
//
// Each prediction and ground-truth label is rendered to text and counted with
// counter. The primary [Result.Value] is the average predicted-token count;
// totals and averages for both predicted and reference tokens are recorded in
// [Result.Values]. Empty input yields a zeroed result rather than a panic.
//
// The metric name embeds the counter's [llm.TokenCounter.Name] (for example
// token_stats[tiktoken:cl100k_base]) and the identity is recorded in
// [Result.Detail], so runs tokenized by incompatible strategies stay distinct in
// provenance and are never compared as if equivalent.
//
// Counting is fallible: if the counter fails to encode a sample, that sample is
// skipped from the totals (its count is not fabricated) and the number of skipped
// samples is recorded as counter_errors in [Result.Values] and [Result.Detail].
// Averages are taken over the samples that counted successfully.
//
// TokenStats is descriptive: its counts summarize usage rather than measure
// quality, so [Result.Direction] is [bench.Neutral] and run comparison skips it
// for regression classification.
//
// TokenStats returns an error if counter is nil (including a typed-nil interface
// value), since a metric with no counter cannot produce meaningful counts.
func TokenStats[L comparable](counter llm.TokenCounter) (Metric[L], error) {
	if isNilCounter(counter) {
		return nil, apperrors.New(apperrors.ErrCodeInvalidInput,
			"token_stats: TokenStats requires a non-nil llm.TokenCounter",
			http.StatusBadRequest)
	}
	name := fmt.Sprintf("%s[%s]", tokenStatsBaseName, counter.Name())
	return &tokenStats[L]{counter: counter, name: name}, nil
}

// isNilCounter reports whether counter is nil, including a typed-nil interface
// value (a nil *T stored in the interface) that a plain counter == nil check
// misses and that would otherwise panic when Compute dispatches through it.
func isNilCounter(counter llm.TokenCounter) bool {
	if counter == nil {
		return true
	}
	v := reflect.ValueOf(counter)
	switch v.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}

type tokenStats[L comparable] struct {
	counter llm.TokenCounter
	name    string
}

func (m *tokenStats[L]) Name() string { return m.name }

func (m *tokenStats[L]) Compute(scored []bench.ScoredSample[L]) Result {
	if len(scored) == 0 {
		return m.result(0, 0, 0, 0)
	}

	predictedTotal := 0
	referenceTotal := 0
	counted := 0
	errors := 0
	for _, s := range scored {
		predicted, perr := m.counter.Count(fmt.Sprintf("%v", s.Prediction.Label))
		reference, rerr := m.counter.Count(fmt.Sprintf("%v", s.Sample.Label))
		if perr != nil || rerr != nil {
			errors++
			continue
		}
		predictedTotal += predicted
		referenceTotal += reference
		counted++
	}
	return m.result(predictedTotal, referenceTotal, counted, errors)
}

func (m *tokenStats[L]) result(predictedTotal, referenceTotal, counted, errors int) Result {
	var predictedAvg, referenceAvg float64
	if counted > 0 {
		predictedAvg = float64(predictedTotal) / float64(counted)
		referenceAvg = float64(referenceTotal) / float64(counted)
	}
	detail := map[string]string{"counter": m.counter.Name()}
	if errors > 0 {
		detail["counter_errors"] = fmt.Sprintf("%d", errors)
	}
	return Result{
		Name:  m.name,
		Value: predictedAvg,
		Values: map[string]float64{
			"predicted_tokens_total": float64(predictedTotal),
			"predicted_tokens_avg":   predictedAvg,
			"reference_tokens_total": float64(referenceTotal),
			"reference_tokens_avg":   referenceAvg,
			"counter_errors":         float64(errors),
		},
		Detail:    detail,
		Direction: bench.Neutral,
	}
}
