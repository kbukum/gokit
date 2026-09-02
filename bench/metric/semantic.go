package metric

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/vector"
	"github.com/kbukum/gokit/bench"
	"github.com/kbukum/gokit/embedding"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/resilience"
)

const (
	// semanticBaseName is the metric name stem; the model identity is appended.
	semanticBaseName = "semantic_similarity"
	// defaultSemanticThreshold is the cosine similarity at or above which a
	// prediction counts as a match for the reported match rate.
	defaultSemanticThreshold = 0.8
	// defaultSemanticBatchSize bounds how many texts are embedded per request.
	defaultSemanticBatchSize = 32
	// defaultSemanticTimeout is the per-call embedding timeout on the default
	// resilience policy; the baseline requires a timeout on every remote call.
	// Override the whole policy with WithSemanticPolicy, or just the timeout with
	// WithSemanticTimeout (a non-positive duration is ignored, keeping this bound
	// so a stalled provider can never hang a run).
	defaultSemanticTimeout = 30 * time.Second
)

// SemanticSimilarity scores each prediction against its reference by embedding both with an injected [embedding.Provider] and taking their cosine similarity (via [vector.CosineSimilarity]). The primary [Result.Value] and avg_similarity in [Result.Values] report the mean similarity; match_rate reports the fraction of samples at or above the threshold. The model identity, the configured threshold, and the evaluated sample count are recorded in [Result.Detail] rather than [Result.Values]: the threshold is a configuration input and the sample count a run summary, not quality signals that run comparison should score as a regression or improvement.
//
// Texts are drawn from each label via the extractor (default: fmt %v) and embedded one batch per provider call via [embedding.Provider.Execute], each routed through the canonical [resilience.Policy] (default: a per-call timeout) so a large run is not scored against a single dataset-wide deadline; every call honors cancellation. Each response is validated: its embeddings must form an index permutation of the batch, so a reordered or malformed response is rejected rather than silently mispairing vectors. Failures are typed, never panics: a malformed or non-finite response surfaces as an external-service [apperrors.AppError], a per-call timeout or cancellation as a timeout/canceled AppError, and a dimension mismatch as an invalid-input AppError — all preserving the cause. Empty input yields a zeroed result without calling the provider.
//
// The metric name embeds the model identity built from provider, name, and version, and the configured threshold — for example semantic_similarity[openai/text-embedding-3-small:t0.8] — so runs scored by different embedding models, versions, or thresholds stay distinct in provenance and are never joined as compatible by name alone: match_rate is a fraction at a fixed cutoff, so comparing it across thresholds would be unsound. The provider name is used only when the model carries no identity metadata.
//
// The configured threshold must be a finite cosine similarity in [-1, 1]; SemanticSimilarity returns an invalid-input error otherwise. It also returns an error if provider is nil (including a typed-nil interface value), since a semantic metric with no provider cannot score.
func SemanticSimilarity[L comparable](provider embedding.Provider, model ai.Model, opts ...SemanticOption[L]) (ContextMetric[L], error) {
	if isNilProvider(provider) {
		return nil, apperrors.New(apperrors.ErrCodeInvalidInput,
			"semantic_similarity: SemanticSimilarity requires a non-nil embedding.Provider",
			http.StatusBadRequest)
	}
	m := &semanticSimilarity[L]{
		provider:  provider,
		model:     model,
		threshold: defaultSemanticThreshold,
		batchSize: defaultSemanticBatchSize,
		policy:    resilience.NewPolicy().WithTimeout(defaultSemanticTimeout),
		extract:   func(l L) string { return fmt.Sprintf("%v", l) },
	}
	for _, o := range opts {
		o(m)
	}
	if err := validateThreshold(m.threshold); err != nil {
		return nil, err
	}
	m.name = fmt.Sprintf("%s[%s:t%s]", semanticBaseName, modelIdentity(model, provider), formatThreshold(m.threshold))
	return m, nil
}

// validateThreshold rejects a threshold that is not a finite cosine similarity in [-1, 1]. A non-finite threshold would be copied into [Result.Values] and break JSON persistence and comparison, so it is caught at construction as a typed invalid-input error.
func validateThreshold(threshold float64) error {
	return validateThresholdRange(semanticBaseName, threshold, -1, 1)
}

// SemanticOption configures a [SemanticSimilarity] metric.
type SemanticOption[L comparable] func(*semanticSimilarity[L])

// WithSemanticThreshold sets the cosine similarity at or above which a prediction counts toward match_rate. The default is 0.8.
func WithSemanticThreshold[L comparable](threshold float64) SemanticOption[L] {
	return func(m *semanticSimilarity[L]) { m.threshold = threshold }
}

// WithSemanticTimeout sets the per-call embedding timeout on the resilience policy, leaving any other configured primitives (retries, circuit breaker, …) intact. A non-positive duration is ignored, preserving the bounded default so a stalled provider can never hang a run; to run without a per-call timeout, pass an explicit [WithSemanticPolicy].
func WithSemanticTimeout[L comparable](d time.Duration) SemanticOption[L] {
	return func(m *semanticSimilarity[L]) {
		if d > 0 {
			m.policy = m.policy.WithTimeout(d)
		}
	}
}

// WithSemanticPolicy routes every embedding provider call through the given canonical [resilience.Policy] instead of a bespoke timeout, so timeouts, bounded retries, and circuit breaking share one configurable seam. The default is a per-call 30-second timeout with no retries; embedding is idempotent, so a caller may add bounded jittered retries. A nil policy is ignored.
func WithSemanticPolicy[L comparable](p *resilience.Policy) SemanticOption[L] {
	return func(m *semanticSimilarity[L]) {
		if p != nil {
			m.policy = p
		}
	}
}

// WithSemanticBatchSize bounds how many texts are embedded per request. Values < 1 keep the default.
func WithSemanticBatchSize[L comparable](n int) SemanticOption[L] {
	return func(m *semanticSimilarity[L]) {
		if n >= 1 {
			m.batchSize = n
		}
	}
}

// WithSemanticExtractor sets how a label is rendered to text before embedding. The default renders with fmt %v. A nil extractor is ignored.
func WithSemanticExtractor[L comparable](fn func(L) string) SemanticOption[L] {
	return func(m *semanticSimilarity[L]) {
		if fn != nil {
			m.extract = fn
		}
	}
}

type semanticSimilarity[L comparable] struct {
	provider  embedding.Provider
	model     ai.Model
	name      string
	threshold float64
	batchSize int
	policy    *resilience.Policy
	extract   func(L) string
}

func (m *semanticSimilarity[L]) Name() string { return m.name }

func (m *semanticSimilarity[L]) Compute(ctx context.Context, scored []bench.ScoredSample[L]) (Result, error) {
	if len(scored) == 0 {
		return m.zeroed(), nil
	}

	// Interleave prediction/reference so pair i occupies positions 2i, 2i+1.
	texts := make([]string, 0, len(scored)*2)
	for _, s := range scored {
		texts = append(texts, m.extract(s.Prediction.Label), m.extract(s.Sample.Label))
	}

	vectors, err := m.embed(ctx, texts)
	if err != nil {
		return Result{}, err
	}
	if len(vectors) != len(texts) {
		return Result{}, apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("semantic_similarity: provider returned %d embeddings for %d inputs", len(vectors), len(texts)),
			http.StatusInternalServerError)
	}

	var sum float64
	matches := 0
	for i := range scored {
		sim, err := vector.CosineSimilarity(vectors[2*i], vectors[2*i+1])
		if err != nil {
			return Result{}, m.similarityError(i, err)
		}
		s := float64(sim)
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
			"avg_similarity": avg,
			"match_rate":     float64(matches) / n,
		},
		Detail: map[string]any{"model": modelIdentity(m.model, m.provider), "threshold": m.threshold, "samples": len(scored)},
	}, nil
}

// similarityError types a [vector.CosineSimilarity] failure by cause: a non-finite component is the untrusted provider's fault (external service), while a dimension mismatch is invalid input. Both preserve the cause.
func (m *semanticSimilarity[L]) similarityError(sample int, err error) error {
	if errors.Is(err, vector.ErrNonFinite) {
		return apperrors.New(apperrors.ErrCodeExternalService,
			fmt.Sprintf("semantic_similarity: sample %d embedding has a non-finite component", sample),
			http.StatusBadGateway).WithCause(err)
	}
	return apperrors.New(apperrors.ErrCodeInvalidInput,
		fmt.Sprintf("semantic_similarity: sample %d embedding dimension mismatch", sample),
		http.StatusBadRequest).WithCause(err)
}

// embed embeds texts in batches, returning the vectors in input order. Each batch is one provider call routed through the resilience policy (default: a per-call timeout), so a large run is not scored against a single dataset-wide deadline; every call honors cancellation.
func (m *semanticSimilarity[L]) embed(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += m.batchSize {
		end := min(start+m.batchSize, len(texts))
		chunk, err := m.embedChunk(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, chunk...)
	}
	return vectors, nil
}

// embedChunk embeds one batch through the resilience policy and returns its vectors reindexed to input order.
func (m *semanticSimilarity[L]) embedChunk(ctx context.Context, texts []string) ([][]float32, error) {
	inputs := make([]embedding.EmbedInput, 0, len(texts))
	for _, t := range texts {
		inputs = append(inputs, embedding.Text{Text: t})
	}
	req := embedding.EmbedRequest{Model: m.model, Inputs: inputs}

	resp, err := resilience.Execute(ctx, m.policy, func(callCtx context.Context) (embedding.EmbedResponse, error) {
		return m.provider.Execute(callCtx, req)
	})
	if err != nil {
		return nil, providerError(err)
	}
	return orderedVectors(resp, len(texts))
}

// providerError classifies an embedding call failure by cause so consumers receive the actionable code: the metric's own timeout and cancellation surface as timeout/canceled rather than being blanket-labeled external-service. All preserve the cause.
func providerError(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return apperrors.Timeout("semantic_similarity embedding").WithCause(err)
	case errors.Is(err, context.Canceled):
		return apperrors.Canceled("semantic_similarity embedding").WithCause(err)
	default:
		return apperrors.New(apperrors.ErrCodeExternalService,
			"semantic_similarity: embedding provider failed", http.StatusBadGateway).WithCause(err)
	}
}

// orderedVectors returns the response vectors reindexed to input order. It requires exactly n embeddings whose [embedding.Embedding.Index] values form a permutation of 0..n-1, so a provider that reorders, duplicates, or drops an index is rejected as an untrusted-response failure rather than silently mispairing prediction and reference vectors. Each vector must also be non-empty with a [embedding.Embedding.Dimensions] equal to its length, so a zero-value or dimension-inconsistent embedding (for example an entry an adapter preallocated for a missing response item) is rejected rather than scored as a spurious similarity.
func orderedVectors(resp embedding.EmbedResponse, n int) ([][]float32, error) {
	embeds := resp.Embeddings
	if len(embeds) != n {
		return nil, invalidResponse(fmt.Sprintf("provider returned %d embeddings for %d inputs", len(embeds), n))
	}

	out := make([][]float32, n)
	seen := make([]bool, n)
	for i := range embeds {
		idx := embeds[i].Index
		if idx < 0 || idx >= n || seen[idx] {
			return nil, invalidResponse(fmt.Sprintf("provider returned out-of-range or duplicate embedding index %d for %d inputs", idx, n))
		}
		vec := embeds[i].Vector
		if len(vec) == 0 || embeds[i].Dimensions != len(vec) {
			return nil, invalidResponse(fmt.Sprintf("embedding at index %d has an empty or dimension-inconsistent vector (dimensions %d, length %d)", idx, embeds[i].Dimensions, len(vec)))
		}
		seen[idx] = true
		out[idx] = vec
	}
	return out, nil
}

// invalidResponse types an untrusted embedding-response failure. The provider is external, so a malformed response is an external-service error.
func invalidResponse(msg string) error {
	return apperrors.New(apperrors.ErrCodeExternalService,
		"semantic_similarity: "+msg, http.StatusBadGateway)
}

func (m *semanticSimilarity[L]) zeroed() Result {
	return Result{
		Name:  m.name,
		Value: 0,
		Values: map[string]float64{
			"avg_similarity": 0,
			"match_rate":     0,
		},
		Detail: map[string]any{"model": modelIdentity(m.model, m.provider), "threshold": m.threshold, "samples": 0},
	}
}

// modelIdentity builds a stable, escaped identity for the metric name and detail from the embedding model's provider, name, and version — for example openai/text-embedding-3-small@v1 — so runs scored by different models or versions stay distinct in provenance and are never joined as compatible by name alone. It falls back to the provider name only when the model carries no provider, name, or version metadata. Separator characters in each part are escaped so the identity is unambiguous.
func modelIdentity(model ai.Model, provider embedding.Provider) string {
	prov := string(model.Provider)
	if prov == "" && model.Name == "" && model.Version == "" {
		return escapeIdentity(provider.Name())
	}
	id := escapeIdentity(model.Name)
	if prov != "" {
		id = escapeIdentity(prov) + "/" + id
	}
	if model.Version != "" {
		id += "@" + escapeIdentity(model.Version)
	}
	return id
}

// isNilProvider reports whether provider is nil, including a typed-nil interface value (a nil *T stored in the interface) that a plain provider == nil check misses and that would otherwise panic when Compute dispatches through it.
func isNilProvider(provider embedding.Provider) bool {
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
