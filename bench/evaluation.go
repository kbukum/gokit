package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/kbukum/gokit/util"
)

type completedObservation struct {
	index  int
	record Record
	err    error
}

func (r *BenchRunner[L]) warmup(ctx context.Context, dataset Dataset[L], b branch[L]) (failures int, err error) {
	if r.cfg.warmup == 0 {
		return 0, nil
	}
	it, err := dataset.Iterator(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, it.Close()) }()
	for range r.cfg.warmup {
		sample, ok, err := it.Next(ctx)
		if err != nil {
			return failures, err
		}
		if !ok {
			break
		}
		if len(sample.Input) > r.cfg.limits.MaxRecordBytes {
			return failures, &RunError{Outcome: OutcomeLimitExceeded, Cause: fmt.Errorf("warmup input exceeds limit")}
		}
		record, err := r.execute(ctx, b, sample, 0, 0)
		if err != nil {
			return failures, err
		}
		if record.Error != "" {
			failures++
		}
		if err := ctx.Err(); err != nil {
			return failures, err
		}
	}
	return failures, nil
}

func (r *BenchRunner[L]) dispatch(ctx context.Context, dataset Dataset[L], b branch[L], branchIndex int, w *runWriter) (info DatasetInfo, digest string, err error) {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	it, err := dataset.Iterator(ctx)
	if err != nil {
		return info, "", err
	}
	defer func() { err = errors.Join(err, it.Close()) }()
	done := make(chan completedObservation, r.cfg.concurrency)
	sem := make(chan struct{}, r.cfg.concurrency)
	pending := make(map[int]completedObservation)
	h := util.NewContentHasher()
	info.LabelDistribution = make(map[string]int)
	var sample Sample[L]
	admitted, committed, repeat := 0, 0, r.cfg.repeats
	eof := false
	for !eof || committed < admitted {
		var slot chan struct{}
		if !eof && admitted-committed < 2*r.cfg.concurrency {
			slot = sem
		}
		select {
		case <-ctx.Done():
			return info, "", ctx.Err()
		case slot <- struct{}{}:
			if ctxErr := ctx.Err(); ctxErr != nil {
				<-sem
				return info, "", ctxErr
			}
			if repeat == r.cfg.repeats {
				var ok bool
				sample, ok, err = it.Next(ctx)
				if err != nil {
					<-sem
					return info, "", err
				}
				if !ok {
					<-sem
					eof = true
					continue
				}
				if info.SampleCount >= r.cfg.limits.MaxSamples || len(sample.Input) > r.cfg.limits.MaxRecordBytes {
					<-sem
					return info, "", &RunError{Outcome: OutcomeLimitExceeded, Cause: fmt.Errorf("dataset exceeds sample/input limit")}
				}
				label, err := roundTripLabel(sample.Label)
				if err != nil {
					<-sem
					return info, "", err
				}
				if err := hashSample(h, sample, label); err != nil {
					<-sem
					return info, "", err
				}
				info.SampleCount++
				addLabel(&info, displayLabel(label), r.cfg.limits.MaxLabels)
				repeat = 0
			}
			input := sample
			input.Input = bytes.Clone(sample.Input)
			idx, sampleIndex, repeatIndex := admitted, info.SampleCount-1, repeat
			admitted++
			repeat++
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { <-sem }()
				record, err := r.execute(ctx, b, input, sampleIndex, repeatIndex)
				select {
				case done <- completedObservation{index: idx, record: record, err: err}:
				case <-ctx.Done():
				}
			}()
		case completion := <-done:
			pending[completion.index] = completion
			for {
				next, ok := pending[committed]
				if !ok {
					break
				}
				if next.err != nil {
					return info, "", next.err
				}
				if err := w.append(ctx, branchIndex, next.record); err != nil {
					return info, "", err
				}
				delete(pending, committed)
				committed++
			}
		}
	}
	if info.SampleCount == 0 {
		return info, "", fmt.Errorf("bench: dataset is empty")
	}
	if err := w.flush(ctx); err != nil {
		return info, "", err
	}
	return info, finishDatasetHash(h, info.SampleCount), ctx.Err()
}

func addLabel(info *DatasetInfo, label string, maxLabels int) {
	if info.LabelDistribution == nil {
		return
	}
	info.LabelDistribution[label]++
	if len(info.LabelDistribution) > maxLabels {
		info.LabelDistribution = nil
		info.DistributionOmitted = true
	}
}

func roundTripLabel[L comparable](label L) ([]byte, error) {
	data, err := json.Marshal(label)
	if err != nil {
		return nil, err
	}
	var decoded L
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, err
	}
	if decoded != label {
		return nil, fmt.Errorf("bench: label does not round-trip to an equal value")
	}
	return data, nil
}

func (r *BenchRunner[L]) execute(ctx context.Context, b branch[L], sample Sample[L], index, repeat int) (Record, error) {
	ctx, cancel := context.WithTimeout(ctx, r.cfg.limits.SampleTimeout)
	defer cancel()
	start := r.cfg.clock.Now()
	o := &observation{clock: r.cfg.clock, start: start, usage: TokenUsage{Source: UsageUnavailable}}
	pred, evalErr := executeSafely(ctx, b.evaluator, sample.Input, o)
	elapsed := r.cfg.clock.Now().Sub(start)
	if evalErr == nil {
		evalErr = ctx.Err()
	}
	if evalErr != nil {
		pred = Prediction[L]{}
	}
	label, err := roundTripLabel(sample.Label)
	if err != nil {
		return Record{}, err
	}
	predicted, err := roundTripLabel(pred.Label)
	if err != nil {
		return Record{}, err
	}
	o.mu.Lock()
	record := Record{ID: sample.ID, SampleIndex: index, Repeat: repeat, Label: label, Predicted: predicted, Score: pred.Score, Correct: evalErr == nil && pred.Label == sample.Label, Latency: elapsed, TTFT: o.ttft, Usage: o.usage}
	o.mu.Unlock()
	if evalErr != nil {
		record.Error = evalErr.Error()
	}
	if record.Latency < 0 || record.TTFT != nil && (*record.TTFT < 0 || *record.TTFT > elapsed) {
		return Record{}, fmt.Errorf("bench: invalid observation timing")
	}
	if record.Usage.Input < 0 || record.Usage.Output < 0 {
		return Record{}, fmt.Errorf("bench: negative token usage")
	}
	switch record.Usage.Source {
	case UsageUnavailable, UsageReported:
	case UsageCounted:
		if record.Usage.Tokenizer == "" {
			return Record{}, fmt.Errorf("bench: counted usage requires tokenizer identity")
		}
	default:
		return Record{}, fmt.Errorf("bench: unknown usage source %q", record.Usage.Source)
	}
	return record, nil
}

func executeSafely[L comparable](ctx context.Context, evaluator Evaluator[L], input []byte, observer Observer) (prediction Prediction[L], err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("bench: evaluator panic: %v", recovered)
		}
	}()
	if observed, ok := evaluator.(ObservedEvaluator[L]); ok {
		return observed.ExecuteObserved(ctx, input, observer)
	}
	return evaluator.Execute(ctx, input)
}
