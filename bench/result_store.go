package bench

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/kbukum/gokit/stream"
	"github.com/kbukum/gokit/util"
)

// ResultStore owns per-run quotas, ordered segments, and summary commit markers.
type ResultStore struct {
	objects      ObjectStore
	mu           sync.Mutex
	active       map[string]bool
	summaryLimit int64
	recordLimit  int64
}

// StoreOption configures trusted read budgets independently of stored data.
type StoreOption func(*ResultStore)

// WithSummaryReadLimit changes the summary read ceiling; run limits cannot exceed it.
func WithSummaryReadLimit(limit int64) StoreOption {
	return func(s *ResultStore) { s.summaryLimit = limit }
}

// WithRecordReadLimit changes the trusted maximum record-segment read size.
func WithRecordReadLimit(limit int64) StoreOption {
	return func(s *ResultStore) { s.recordLimit = limit }
}

// NewResultStore wraps objects with bounded run persistence.
func NewResultStore(objects ObjectStore, opts ...StoreOption) *ResultStore {
	s := &ResultStore{objects: objects, active: make(map[string]bool), summaryLimit: DefaultLimits().MaxSummaryBytes, recordLimit: int64(DefaultLimits().MaxRecordBytes)}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type runWriter struct {
	store    *ResultStore
	id       string
	limits   Limits
	bytes    int64
	segments []int
	buffer   bytes.Buffer
	branch   int
}

func summaryKey(id string) string { return "runs/" + id + ".json" }
func recordKey(id string, branch, segment int) string {
	return fmt.Sprintf("records/%s/%d/%08d.jsonl", id, branch, segment)
}

func (s *ResultStore) begin(ctx context.Context, id string, branches int, limits Limits) (*runWriter, error) {
	if util.IsNil(s.objects) || s.summaryLimit < 1 || s.recordLimit < 1 || limits.MaxSummaryBytes > s.summaryLimit || int64(max(limits.MaxRecordBytes, limits.SegmentBytes)) > s.recordLimit {
		return nil, fmt.Errorf("bench: object store and compatible positive read limits required")
	}
	if err := validateRunID(id); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[id] {
		return nil, fmt.Errorf("bench: run %q is active", id)
	}
	keys, err := s.objects.List(ctx, summaryKey(id))
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		if key == summaryKey(id) {
			return nil, fmt.Errorf("bench: run %q already exists", id)
		}
	}
	keys, err = s.objects.List(ctx, "records/"+id+"/")
	if err != nil {
		return nil, err
	}
	if len(keys) != 0 {
		return nil, fmt.Errorf("bench: uncommitted records already exist for %q", id)
	}
	s.active[id] = true
	return &runWriter{store: s, id: id, limits: limits, segments: make([]int, branches)}, nil
}

func (w *runWriter) release() {
	w.store.mu.Lock()
	defer w.store.mu.Unlock()
	delete(w.store.active, w.id)
}

func (w *runWriter) put(ctx context.Context, key string, data []byte) error {
	if int64(len(data)) > w.limits.MaxRunBytes-w.bytes {
		return &RunError{Outcome: OutcomeLimitExceeded, Cause: fmt.Errorf("run quota %d bytes exceeded", w.limits.MaxRunBytes)}
	}
	if err := w.store.objects.Put(ctx, key, data); err != nil {
		return &RunError{Outcome: OutcomeStorageFailed, Cause: err}
	}
	w.bytes += int64(len(data))
	return nil
}

func (w *runWriter) append(ctx context.Context, branch int, record Record) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(data)+1 > w.limits.MaxRecordBytes {
		return &RunError{Outcome: OutcomeLimitExceeded, Cause: fmt.Errorf("record exceeds %d bytes", w.limits.MaxRecordBytes)}
	}
	if w.branch != branch || w.buffer.Len()+len(data)+1 > w.limits.SegmentBytes {
		if err := w.flush(ctx); err != nil {
			return err
		}
	}
	w.branch = branch
	w.buffer.Write(data)
	w.buffer.WriteByte('\n')
	return nil
}

func (w *runWriter) flush(ctx context.Context) error {
	if w.buffer.Len() == 0 {
		return nil
	}
	if err := w.put(ctx, recordKey(w.id, w.branch, w.segments[w.branch]), w.buffer.Bytes()); err != nil {
		return err
	}
	w.segments[w.branch]++
	w.buffer.Reset()
	return nil
}

func (w *runWriter) commit(ctx context.Context, result *RunResult) error {
	if err := w.flush(ctx); err != nil {
		return err
	}
	result.RecordSegments = slices.Clone(w.segments)
	result.RecordReadLimit = int64(max(w.limits.SegmentBytes, w.limits.MaxRecordBytes))
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if int64(len(data)) > w.limits.MaxSummaryBytes {
		return &RunError{Outcome: OutcomeLimitExceeded, Cause: fmt.Errorf("summary exceeds read limit")}
	}
	return w.put(ctx, summaryKey(w.id), data)
}

func (s *ResultStore) Load(ctx context.Context, id string) (*RunResult, error) {
	if err := validateRunID(id); err != nil {
		return nil, err
	}
	data, err := s.objects.Get(ctx, summaryKey(id), s.summaryLimit)
	if err != nil {
		return nil, err
	}
	var result RunResult
	var envelope struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	if envelope.Version != SchemaVersion {
		return nil, fmt.Errorf("bench: unsupported result schema %q", envelope.Version)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	if result.ID != id {
		return nil, fmt.Errorf("bench: summary ID mismatch")
	}
	return &result, nil
}

func (s *ResultStore) Delete(ctx context.Context, id string) error {
	if err := validateRunID(id); err != nil {
		return err
	}
	if err := s.objects.Delete(ctx, summaryKey(id)); err != nil {
		return err
	}
	keys, err := s.objects.List(ctx, "records/"+id+"/")
	if err != nil {
		return err
	}
	var errs []error
	for _, key := range keys {
		if err := s.objects.Delete(ctx, key); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *ResultStore) List(ctx context.Context, opts ...ListOption) ([]RunSummary, error) {
	params := ResolveListOptions(opts...)
	keys, err := s.objects.List(ctx, "runs/")
	if err != nil {
		return nil, err
	}
	var out []RunSummary
	for _, key := range keys {
		if !strings.HasSuffix(key, ".json") {
			continue
		}
		result, err := s.Load(ctx, strings.TrimSuffix(strings.TrimPrefix(key, "runs/"), ".json"))
		if err != nil {
			return nil, err
		}
		if params.Tag != "" && result.Tag != params.Tag || params.Dataset != "" && result.Dataset.Name != params.Dataset {
			continue
		}
		var f1 float64
		for i := range result.Metrics {
			m := &result.Metrics[i]
			if value, ok := m.Values["f1"]; ok {
				f1 = value
				break
			}
			if m.Name == "classification" || m.Name == "multi_class_classification" {
				f1 = m.Value
				break
			}
		}
		out = append(out, RunSummary{ID: result.ID, Timestamp: result.Timestamp, Tag: result.Tag, Dataset: result.Dataset.Name, F1: f1})
	}
	slices.SortFunc(out, func(a, b RunSummary) int { return b.Timestamp.Compare(a.Timestamp) })
	if params.Limit > 0 && len(out) > params.Limit {
		out = out[:params.Limit]
	}
	return out, nil
}

func (s *ResultStore) Latest(ctx context.Context) (*RunResult, error) {
	list, err := s.List(ctx, WithLimit(1))
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("bench: no results found")
	}
	return s.Load(ctx, list[0].ID)
}

// Records opens an ordered branch iterator from a committed run.
func (s *ResultStore) Records(ctx context.Context, id string, branch int) (stream.Iterator[Record], error) {
	result, err := s.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	if branch < 0 || branch >= len(result.RecordSegments) {
		return nil, fmt.Errorf("bench: invalid branch %d", branch)
	}
	if result.RecordSegments[branch] < 0 || result.RecordReadLimit < 1 || result.RecordReadLimit > s.recordLimit {
		return nil, fmt.Errorf("bench: invalid record manifest or read limit")
	}
	return s.records(id, branch, result.RecordSegments[branch], result.RecordReadLimit), nil
}

func (s *ResultStore) records(id string, branch, segments int, limit int64) *recordIterator {
	return &recordIterator{store: s, id: id, branch: branch, segments: segments, limit: limit}
}

type recordIterator struct {
	store                  *ResultStore
	id                     string
	branch, segments, next int
	limit                  int64
	scanner                *bufio.Scanner
	closed                 bool
}

func (it *recordIterator) Next(ctx context.Context) (Record, bool, error) {
	for !it.closed {
		if err := ctx.Err(); err != nil {
			return Record{}, false, err
		}
		if it.scanner != nil {
			if it.scanner.Scan() {
				var record Record
				err := json.Unmarshal(it.scanner.Bytes(), &record)
				return record, err == nil, err
			}
			if err := it.scanner.Err(); err != nil {
				return Record{}, false, err
			}
			it.scanner = nil
		}
		if it.next >= it.segments {
			break
		}
		data, err := it.store.objects.Get(ctx, recordKey(it.id, it.branch, it.next), it.limit)
		if err != nil {
			return Record{}, false, &RunError{Outcome: OutcomeStorageFailed, Cause: err}
		}
		it.next++
		it.scanner = bufio.NewScanner(bytes.NewReader(data))
		it.scanner.Buffer(make([]byte, min(4096, len(data))), int(it.limit)+1)
	}
	return Record{}, false, nil
}

func (it *recordIterator) Close() error { it.closed = true; it.scanner = nil; return nil }

// Observations decodes persisted labels into the consumer's comparable type.
func Observations[L comparable](ctx context.Context, store *ResultStore, id string, branch int) (stream.Iterator[ScoredSample[L]], error) {
	records, err := store.Records(ctx, id, branch)
	if err != nil {
		return nil, err
	}
	return &observationIterator[L]{records: records}, nil
}

type observationIterator[L comparable] struct{ records stream.Iterator[Record] }

func (it *observationIterator[L]) Next(ctx context.Context) (ScoredSample[L], bool, error) {
	record, ok, err := it.records.Next(ctx)
	if !ok || err != nil {
		return ScoredSample[L]{}, false, err
	}
	scored, err := decodeObservation[L](record)
	return scored, err == nil, err
}
func (it *observationIterator[L]) Close() error { return it.records.Close() }

func decodeObservation[L comparable](record Record) (ScoredSample[L], error) {
	var s ScoredSample[L]
	s.Sample.ID = record.ID
	s.Prediction.SampleID = record.ID
	s.Prediction.Score = record.Score
	if err := json.Unmarshal(record.Label, &s.Sample.Label); err != nil {
		return s, fmt.Errorf("bench: decode label: %w", err)
	}
	if err := json.Unmarshal(record.Predicted, &s.Prediction.Label); err != nil {
		return s, fmt.Errorf("bench: decode prediction: %w", err)
	}
	s.Error = record.Error
	return s, nil
}

func (w *runWriter) iterator(branch int) *recordIterator {
	return w.store.records(w.id, branch, w.segments[branch], int64(max(w.limits.SegmentBytes, w.limits.MaxRecordBytes)))
}

func (w *runWriter) String() string { return w.id + ":" + strconv.FormatInt(w.bytes, 10) }
