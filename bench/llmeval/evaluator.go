package llmeval

import (
	"context"
	"fmt"
	"strings"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/bench"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/util"
)

// Config injects request and prediction mapping, with finite output limits.
type Config[L comparable] struct {
	Request        func([]byte) llm.CompletionRequest
	Prediction     func(llm.CompletionResponse) (bench.Prediction[L], error)
	Counter        llm.TokenCounter
	Stream         bool
	MaxOutputBytes int
}

type evaluator[L comparable] struct {
	provider llm.Provider
	cfg      Config[L]
}

// New creates a unary or streaming evaluator with optional counted usage fallback.
func New[L comparable](provider llm.Provider, cfg Config[L]) (bench.Evaluator[L], error) {
	if util.IsNil(provider) || cfg.Request == nil || cfg.Prediction == nil {
		return nil, fmt.Errorf("llmeval: provider and mappers are required")
	}
	if cfg.MaxOutputBytes == 0 {
		cfg.MaxOutputBytes = 1 << 20
	}
	if cfg.MaxOutputBytes < 1 {
		return nil, fmt.Errorf("llmeval: output limit must be positive")
	}
	if cfg.Counter != nil && util.IsNil(cfg.Counter) {
		return nil, fmt.Errorf("llmeval: token counter is typed nil")
	}
	return &evaluator[L]{provider: provider, cfg: cfg}, nil
}
func (e *evaluator[L]) Name() string                         { return e.provider.Name() }
func (e *evaluator[L]) IsAvailable(ctx context.Context) bool { return e.provider.IsAvailable(ctx) }
func (e *evaluator[L]) Execute(ctx context.Context, input []byte) (bench.Prediction[L], error) {
	return e.ExecuteObserved(ctx, input, nil)
}

func (e *evaluator[L]) ExecuteObserved(ctx context.Context, input []byte, observer bench.Observer) (bench.Prediction[L], error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	request := e.cfg.Request(input)
	var response llm.CompletionResponse
	var err error
	var reported bool
	if e.cfg.Stream {
		response, reported, err = e.stream(ctx, request, observer)
	} else {
		response, err = e.provider.Execute(ctx, request)
		reported = response.UsageReported
	}
	if err != nil {
		return bench.Prediction[L]{}, err
	}
	if len(response.Text()) > e.cfg.MaxOutputBytes {
		return bench.Prediction[L]{}, fmt.Errorf("llmeval: output exceeds byte limit")
	}
	if observer != nil {
		if response.UsageReported {
			observer.ReportUsage(usage(response.Usage))
			reported = true
		}
		if !reported && e.cfg.Counter != nil {
			in, err := e.cfg.Counter.Count(string(input))
			if err != nil {
				return bench.Prediction[L]{}, err
			}
			out, err := e.cfg.Counter.Count(response.Text())
			if err != nil {
				return bench.Prediction[L]{}, err
			}
			observer.ReportUsage(bench.TokenUsage{Input: int64(in), Output: int64(out), Source: bench.UsageCounted, Tokenizer: e.cfg.Counter.Name()})
		}
	}
	return e.cfg.Prediction(response)
}

func usage(u ai.Usage) bench.TokenUsage {
	return bench.TokenUsage{Input: int64(u.InputTokens), Output: int64(u.OutputTokens), Source: bench.UsageReported}
}

func (e *evaluator[L]) stream(ctx context.Context, request llm.CompletionRequest, observer bench.Observer) (llm.CompletionResponse, bool, error) {
	events, err := e.provider.Stream(ctx, request)
	if err != nil {
		return llm.CompletionResponse{}, false, err
	}
	reported := false
	var content strings.Builder
	for {
		select {
		case <-ctx.Done():
			return llm.CompletionResponse{}, reported, ctx.Err()
		case event, ok := <-events:
			if !ok {
				return llm.CompletionResponse{}, reported, fmt.Errorf("llmeval: stream closed without MessageComplete")
			}
			switch event := event.(type) {
			case llm.TextDelta:
				if event.Text != "" && observer != nil {
					observer.ContentStarted()
				}
				if len(event.Text) > e.cfg.MaxOutputBytes-content.Len() {
					return llm.CompletionResponse{}, reported, fmt.Errorf("llmeval: output exceeds byte limit")
				}
				content.WriteString(event.Text)
			case llm.UsageDelta:
				reported = true
				if observer != nil {
					observer.ReportUsage(bench.TokenUsage{Input: int64(event.InputTokens), Output: int64(event.OutputTokens), Source: bench.UsageReported})
				}
			case llm.MessageComplete:
				return event.Response, reported, nil
			case ai.Error:
				return llm.CompletionResponse{}, reported, event
			}
		}
	}
}
