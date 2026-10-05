package bench

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"slices"
	"time"
)

// MetricIdentity separates the canonical comparison name from algorithm and config.
type MetricIdentity struct {
	Name    string            `json:"name"`
	Version string            `json:"version"`
	Config  map[string]string `json:"config,omitempty"`
}

// SubjectIdentity describes what was evaluated, not comparison eligibility.
type SubjectIdentity struct {
	Name        string            `json:"name"`
	ModelDigest string            `json:"model_digest"`
	Config      map[string]string `json:"config,omitempty"`
}

// ExecutionIdentity records conditions that affect measurement.
type ExecutionIdentity struct {
	Concurrency   int           `json:"concurrency"`
	Repeats       int           `json:"repeats"`
	Warmup        int           `json:"warmup"`
	SampleTimeout time.Duration `json:"sample_timeout_ns"`
	Seed          uint64        `json:"seed"`
}

// EvaluationIdentity pins every input required for comparable measurements.
type EvaluationIdentity struct {
	Version    string            `json:"version"`
	Dataset    string            `json:"dataset"`
	Metrics    []MetricIdentity  `json:"metrics"`
	Judges     []JudgeProvenance `json:"judges,omitempty"`
	Tokenizers []string          `json:"tokenizers,omitempty"`
	Execution  ExecutionIdentity `json:"execution"`
}

func fingerprint[T any](value T) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

type EligibilityReason string

const (
	UnsupportedIdentityVersion EligibilityReason = "unsupported_identity_version"
	IncompleteIdentity         EligibilityReason = "incomplete_identity"
	DatasetChanged             EligibilityReason = "dataset_changed"
	MetricChanged              EligibilityReason = "metric_changed"
	MetricMissing              EligibilityReason = "metric_missing"
	JudgeChanged               EligibilityReason = "judge_changed"
	TokenizerChanged           EligibilityReason = "tokenizer_changed"
	ExecutionChanged           EligibilityReason = "execution_changed"
)

// Eligibility distinguishes identity completeness from compatibility.
type Eligibility struct {
	Eligible bool                `json:"eligible"`
	Complete bool                `json:"complete"`
	Reasons  []EligibilityReason `json:"reasons,omitempty"`
}

func (e EvaluationIdentity) complete() bool {
	if e.Version == "" || e.Dataset == "" || len(e.Metrics) == 0 || e.Execution.Concurrency < 1 || e.Execution.Repeats < 1 || e.Execution.SampleTimeout <= 0 {
		return false
	}
	for _, m := range e.Metrics {
		if m.Name == "" || m.Version == "" {
			return false
		}
	}
	for i := range e.Judges {
		j := &e.Judges[i]
		if j.Model == "" || j.PromptVersion == "" || j.PromptFingerprint == "" {
			return false
		}
	}
	for _, tokenizer := range e.Tokenizers {
		if tokenizer == "" {
			return false
		}
	}
	return true
}

func compareIdentity(a, b EvaluationIdentity) Eligibility {
	e := Eligibility{Complete: a.complete() && b.complete()}
	if !e.Complete {
		e.Reasons = append(e.Reasons, IncompleteIdentity)
	}
	if a.Version != "1" || b.Version != "1" {
		e.Reasons = append(e.Reasons, UnsupportedIdentityVersion)
	}
	if a.Dataset != b.Dataset {
		e.Reasons = append(e.Reasons, DatasetChanged)
	}
	am, bm := make(map[string]MetricIdentity), make(map[string]MetricIdentity)
	for _, m := range a.Metrics {
		am[m.Name] = m
	}
	for _, m := range b.Metrics {
		bm[m.Name] = m
	}
	missing, changed := false, false
	for name, m := range am {
		n, ok := bm[name]
		missing = missing || !ok
		changed = changed || ok && !reflect.DeepEqual(m, n)
	}
	for name := range bm {
		_, ok := am[name]
		missing = missing || !ok
	}
	if missing {
		e.Reasons = append(e.Reasons, MetricMissing)
	}
	if changed {
		e.Reasons = append(e.Reasons, MetricChanged)
	}
	if !reflect.DeepEqual(a.Judges, b.Judges) {
		e.Reasons = append(e.Reasons, JudgeChanged)
	}
	if !slices.Equal(a.Tokenizers, b.Tokenizers) {
		e.Reasons = append(e.Reasons, TokenizerChanged)
	}
	if a.Execution != b.Execution {
		e.Reasons = append(e.Reasons, ExecutionChanged)
	}
	e.Eligible = len(e.Reasons) == 0
	return e
}
