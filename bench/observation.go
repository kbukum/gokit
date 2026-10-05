package bench

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/kbukum/gokit/util"
)

// UsageSource distinguishes missing usage from a reported or counted zero.
type UsageSource string

const (
	UsageUnavailable UsageSource = "unavailable"
	UsageReported    UsageSource = "reported"
	UsageCounted     UsageSource = "counted"
)

// TokenUsage records runtime usage or the identity of an injected counter.
type TokenUsage struct {
	Input     int64       `json:"input"`
	Output    int64       `json:"output"`
	Source    UsageSource `json:"source"`
	Tokenizer string      `json:"tokenizer,omitempty"`
}

// Observer accepts content-only timing and final usage. Calls are concurrency safe.
type Observer interface {
	ContentStarted()
	ReportUsage(TokenUsage)
}

// ObservedEvaluator adds optional runtime observations to an evaluator.
type ObservedEvaluator[L comparable] interface {
	ExecuteObserved(context.Context, []byte, Observer) (Prediction[L], error)
}

type observation struct {
	mu    sync.Mutex
	clock util.Clock
	start time.Time
	ttft  *time.Duration
	usage TokenUsage
}

func (o *observation) ContentStarted() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.ttft == nil {
		d := o.clock.Now().Sub(o.start)
		o.ttft = &d
	}
}

func (o *observation) ReportUsage(usage TokenUsage) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.usage = usage
}

// Record is the non-generic persisted observation. Labels retain their JSON types.
type Record struct {
	ID          string          `json:"id"`
	SampleIndex int             `json:"sample_index"`
	Repeat      int             `json:"repeat"`
	Label       json.RawMessage `json:"label"`
	Predicted   json.RawMessage `json:"predicted"`
	Score       float64         `json:"score"`
	Correct     bool            `json:"correct"`
	Error       string          `json:"error,omitempty"`
	Latency     time.Duration   `json:"latency_ns"`
	TTFT        *time.Duration  `json:"ttft_ns,omitempty"`
	Usage       TokenUsage      `json:"usage"`
}

// LabelText renders a label without losing the stored typed representation.
func (r Record) LabelText() string { return displayLabel(r.Label) }

// PredictedText renders a predicted label.
func (r Record) PredictedText() string { return displayLabel(r.Predicted) }

func displayLabel(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

// Percentiles uses exact nearest ranks and an arithmetic mean, in milliseconds.
type Percentiles struct {
	Count int     `json:"count"`
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
	Mean  float64 `json:"mean"`
}

// PerformanceSummary excludes failed observations from timing and throughput.
type PerformanceSummary struct {
	Observations   int          `json:"observations"`
	Failures       int          `json:"failures"`
	WarmupFailures int          `json:"warmup_failures"`
	WithoutUsage   int          `json:"without_usage"`
	Latency        Percentiles  `json:"latency_ms"`
	TTFT           *Percentiles `json:"ttft_ms,omitempty"`
	InputTokens    int64        `json:"input_tokens"`
	OutputTokens   int64        `json:"output_tokens"`
	OutputTPS      float64      `json:"throughput_output_tps"`
	TotalTPS       float64      `json:"throughput_total_tps"`
}
