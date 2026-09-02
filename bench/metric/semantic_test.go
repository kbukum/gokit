package metric_test

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/bench"
	"github.com/kbukum/gokit/bench/metric"
	"github.com/kbukum/gokit/embedding"
	"github.com/kbukum/gokit/embedding/inmem"
	embedtestutil "github.com/kbukum/gokit/embedding/testutil"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/resilience"
)

func semanticScored(pairs ...[2]string) []bench.ScoredSample[string] {
	scored := make([]bench.ScoredSample[string], len(pairs))
	for i, p := range pairs {
		scored[i] = bench.ScoredSample[string]{
			Sample:     bench.Sample[string]{ID: "s", Label: p[1]},
			Prediction: bench.Prediction[string]{Label: p[0]},
		}
	}
	return scored
}

func newSemantic(t *testing.T, provider embedding.Provider, opts ...metric.SemanticOption[string]) metric.ContextMetric[string] {
	t.Helper()
	m, err := metric.SemanticSimilarity[string](provider, ai.Model{Name: "test-embed"}, opts...)
	if err != nil {
		t.Fatalf("SemanticSimilarity: %v", err)
	}
	return m
}

func TestSemanticSimilarityNameEmbedsModel(t *testing.T) {
	t.Parallel()

	m := newSemantic(t, inmem.New(8))
	if got, want := m.Name(), "semantic_similarity[test-embed:t0.8]"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

// The threshold is folded into the metric name so match_rate — a fraction at a
// fixed cutoff — is never compared across runs that used different thresholds.
func TestSemanticSimilarityThresholdChangesIdentity(t *testing.T) {
	t.Parallel()

	a := newSemantic(t, inmem.New(8), metric.WithSemanticThreshold[string](0.7))
	b := newSemantic(t, inmem.New(8), metric.WithSemanticThreshold[string](0.9))
	if a.Name() == b.Name() {
		t.Errorf("names must differ by threshold, both = %q", a.Name())
	}
	if !strings.Contains(a.Name(), ":t0.7]") {
		t.Errorf("Name() = %q, want threshold t0.7 in identity", a.Name())
	}
}

func TestSemanticSimilarityRejectsNilProvider(t *testing.T) {
	t.Parallel()

	assertInvalidInput := func(t *testing.T, err error) {
		t.Helper()
		appErr, ok := apperrors.AsAppError(err)
		if !ok {
			t.Fatalf("error is not an *apperrors.AppError: %v", err)
		}
		if appErr.Code != apperrors.ErrCodeInvalidInput {
			t.Errorf("Code = %q, want %q", appErr.Code, apperrors.ErrCodeInvalidInput)
		}
		if appErr.HTTPStatus != http.StatusBadRequest {
			t.Errorf("HTTPStatus = %d, want %d", appErr.HTTPStatus, http.StatusBadRequest)
		}
	}

	_, err := metric.SemanticSimilarity[string](nil, ai.Model{})
	if err == nil {
		t.Fatal("expected error for nil provider, got nil")
	}
	assertInvalidInput(t, err)

	var typedNil *inmem.Provider
	_, err = metric.SemanticSimilarity[string](typedNil, ai.Model{})
	if err == nil {
		t.Fatal("expected error for typed-nil provider, got nil")
	}
	assertInvalidInput(t, err)
}

func TestSemanticSimilarityIdenticalTextsScoreOne(t *testing.T) {
	t.Parallel()

	m := newSemantic(t, inmem.New(8))
	res, err := m.Compute(context.Background(), semanticScored(
		[2]string{"hello world", "hello world"},
		[2]string{"the cat sat", "the cat sat"},
	))
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if res.Value < 0.999 {
		t.Errorf("avg_similarity = %f, want ~1.0 for identical texts", res.Value)
	}
	if res.Values["match_rate"] != 1.0 {
		t.Errorf("match_rate = %f, want 1.0", res.Values["match_rate"])
	}
	if got := res.Detail.(map[string]any)["samples"]; got != 2 {
		t.Errorf("Detail[samples] = %v, want 2", got)
	}
	if res.Direction != bench.HigherIsBetter {
		t.Errorf("Direction = %v, want HigherIsBetter (semantic similarity measures quality)", res.Direction)
	}
}

func TestSemanticSimilarityThresholdMatchRate(t *testing.T) {
	t.Parallel()

	// One identical pair (similarity 1.0 ≥ threshold) and one mismatched pair
	// (low similarity < threshold) ⇒ match_rate 0.5 at a high threshold.
	m := newSemantic(t, inmem.New(8), metric.WithSemanticThreshold[string](0.99))
	res, err := m.Compute(context.Background(), semanticScored(
		[2]string{"identical", "identical"},
		[2]string{"prediction text", "totally different reference"},
	))
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if res.Values["match_rate"] != 0.5 {
		t.Errorf("match_rate = %f, want 0.5", res.Values["match_rate"])
	}
	if _, ok := res.Values["threshold"]; ok {
		t.Error("threshold must not appear in Values: it is a configuration input, not a quality signal")
	}
	if got := res.Detail.(map[string]any)["threshold"]; got != 0.99 {
		t.Errorf("Detail[threshold] = %v, want 0.99", got)
	}
}

func TestSemanticSimilarityBatchBoundarySplitsPairs(t *testing.T) {
	t.Parallel()

	// Small/odd batch sizes split prediction/reference pairs across separate
	// embedding calls; the reassembled vectors must still re-align to the 2i/2i+1
	// pairing, so identical texts score ~1.0 regardless of batch size. Locks the
	// ordering contract against a future provider that reorders across requests.
	for _, batchSize := range []int{1, 3} {
		m := newSemantic(t, inmem.New(8), metric.WithSemanticBatchSize[string](batchSize))
		res, err := m.Compute(context.Background(), semanticScored(
			[2]string{"alpha", "alpha"},
			[2]string{"beta", "beta"},
			[2]string{"gamma", "gamma"},
		))
		if err != nil {
			t.Fatalf("batchSize=%d Compute: %v", batchSize, err)
		}
		if res.Value < 0.999 {
			t.Errorf("batchSize=%d avg_similarity = %f, want ~1.0", batchSize, res.Value)
		}
	}
}

func TestSemanticSimilarityEmptyInputIsZeroedNoCall(t *testing.T) {
	t.Parallel()

	provider := embedtestutil.NewFakeProvider()
	m := newSemantic(t, provider)
	res, err := m.Compute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if res.Value != 0 || res.Detail.(map[string]any)["samples"] != 0 {
		t.Errorf("empty result = %+v, want zeroed", res)
	}
	if provider.Calls() != 0 {
		t.Errorf("provider called %d times on empty input, want 0", provider.Calls())
	}
}

func TestSemanticSimilarityProviderError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("embedding down")
	m := newSemantic(t, embedtestutil.NewFakeProvider(embedtestutil.WithError(sentinel)))
	_, err := m.Compute(context.Background(), semanticScored([2]string{"a", "b"}))
	if err == nil {
		t.Fatal("expected error from failing provider, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error does not preserve cause: %v", err)
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeExternalService {
		t.Errorf("error = %v, want AppError with code %s", err, apperrors.ErrCodeExternalService)
	}
}

func TestSemanticSimilarityDimensionMismatch(t *testing.T) {
	t.Parallel()

	// Pin the prediction and reference texts to vectors of differing lengths so
	// cosine similarity surfaces a dimension-mismatch error, typed not panicked.
	provider := embedtestutil.NewFakeProvider(
		embedtestutil.WithVector("pred", []float32{1, 0}),
		embedtestutil.WithVector("ref", []float32{1, 0, 0, 0}),
	)
	m := newSemantic(t, provider)
	_, err := m.Compute(context.Background(), semanticScored([2]string{"pred", "ref"}))
	if err == nil {
		t.Fatal("expected dimension-mismatch error, got nil")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeInvalidInput {
		t.Errorf("error = %v, want AppError with code %s", err, apperrors.ErrCodeInvalidInput)
	}
}

func TestSemanticSimilarityHonorsCancellation(t *testing.T) {
	t.Parallel()

	provider := embedtestutil.NewFakeProvider(embedtestutil.WithBlockUntilCancel())
	m := newSemantic(t, provider, metric.WithSemanticTimeout[string](0))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := m.Compute(ctx, semanticScored([2]string{"a", "b"}))
	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeCanceled {
		t.Errorf("error = %v, want AppError %s", err, apperrors.ErrCodeCanceled)
	}
}

func TestSemanticSimilarityTimeoutBounds(t *testing.T) {
	t.Parallel()

	provider := embedtestutil.NewFakeProvider(embedtestutil.WithBlockUntilCancel())
	m := newSemantic(t, provider, metric.WithSemanticTimeout[string](10*time.Millisecond))
	_, err := m.Compute(context.Background(), semanticScored([2]string{"a", "b"}))
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeTimeout {
		t.Errorf("error = %v, want AppError %s", err, apperrors.ErrCodeTimeout)
	}
}

// SemanticSimilarity adapts to a bench.RunContextMetric usable with the runner.
func TestSemanticSimilarityAdaptsToRunContextMetric(t *testing.T) {
	t.Parallel()

	rm := metric.AsRunContextMetric[string](newSemantic(t, inmem.New(8)))
	out, err := rm.Compute(context.Background(), semanticScored([2]string{"x", "x"}))
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if out.Name != "semantic_similarity[test-embed:t0.8]" {
		t.Errorf("RunContextMetric name = %q", out.Name)
	}
}

func TestSemanticSimilarityRejectsInvalidThreshold(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		threshold float64
	}{
		{"nan", math.NaN()},
		{"pos_inf", math.Inf(1)},
		{"neg_inf", math.Inf(-1)},
		{"above_one", 1.5},
		{"below_neg_one", -1.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := metric.SemanticSimilarity[string](inmem.New(8), ai.Model{Name: "m"},
				metric.WithSemanticThreshold[string](tc.threshold))
			if err == nil {
				t.Fatalf("threshold %v: expected error, got nil", tc.threshold)
			}
			var appErr *apperrors.AppError
			if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeInvalidInput {
				t.Errorf("threshold %v: error = %v, want AppError %s", tc.threshold, err, apperrors.ErrCodeInvalidInput)
			}
		})
	}
}

func TestSemanticSimilarityIdentityIncludesProviderAndVersion(t *testing.T) {
	t.Parallel()

	// Provider, name, and version compose a stable identity so two models with the
	// same short name but different provider/version never collide in provenance.
	m, err := metric.SemanticSimilarity[string](inmem.New(8),
		ai.Model{Provider: ai.ProviderOpenAI, Name: "text-embedding-3-small", Version: "v1"})
	if err != nil {
		t.Fatalf("SemanticSimilarity: %v", err)
	}
	if got, want := m.Name(), "semantic_similarity[openai/text-embedding-3-small@v1:t0.8]"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestSemanticSimilarityIdentityEscapesSeparators(t *testing.T) {
	t.Parallel()

	// Separator characters in a model name must be escaped so a crafted name
	// cannot forge or collide with another identity's structure.
	m, err := metric.SemanticSimilarity[string](inmem.New(8), ai.Model{Name: "a/b@c]"})
	if err != nil {
		t.Fatalf("SemanticSimilarity: %v", err)
	}
	if got, want := m.Name(), `semantic_similarity[a\/b\@c\]:t0.8]`; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestSemanticSimilarityIdentityFallsBackToProviderName(t *testing.T) {
	t.Parallel()

	// With no model metadata, the provider name identifies the run.
	m, err := metric.SemanticSimilarity[string](
		embedtestutil.NewFakeProvider(embedtestutil.WithName("embed-fake")), ai.Model{})
	if err != nil {
		t.Fatalf("SemanticSimilarity: %v", err)
	}
	if got, want := m.Name(), "semantic_similarity[embed-fake:t0.8]"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestSemanticSimilarityHonorsResponseIndexOrder(t *testing.T) {
	t.Parallel()

	// A provider may return embeddings in a different slice order than the inputs.
	// The metric must reassemble by Embedding.Index, not slice position, or it
	// silently mispairs prediction and reference vectors across samples.
	vecs := map[string][]float32{"same0": {1, 0}, "same1": {0, 1}}
	responder := func(_ context.Context, req embedding.EmbedRequest) (embedding.EmbedResponse, error) {
		all := make([]embedding.Embedding, len(req.Inputs))
		for i, in := range req.Inputs {
			text := in.(embedding.Text).Text
			all[i] = embedding.Embedding{Vector: vecs[text], Dimensions: 2, Index: i}
		}
		// Emit even indices then odd indices: a scrambled slice order with Index
		// preserved. Trusting slice order would pair (same0,same1) and score ~0.
		scrambled := make([]embedding.Embedding, 0, len(all))
		for i := 0; i < len(all); i += 2 {
			scrambled = append(scrambled, all[i])
		}
		for i := 1; i < len(all); i += 2 {
			scrambled = append(scrambled, all[i])
		}
		return embedding.EmbedResponse{Embeddings: scrambled}, nil
	}

	m := newSemantic(t, embedtestutil.NewFakeProvider(embedtestutil.WithResponder(responder)))
	res, err := m.Compute(context.Background(), semanticScored(
		[2]string{"same0", "same0"},
		[2]string{"same1", "same1"},
	))
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if res.Value < 0.999 {
		t.Errorf("avg_similarity = %f, want ~1.0 (index order must reassemble pairs)", res.Value)
	}
}

func TestSemanticSimilarityRejectsMalformedResponse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		responder func(context.Context, embedding.EmbedRequest) (embedding.EmbedResponse, error)
	}{
		{
			name: "wrong_count",
			responder: func(_ context.Context, req embedding.EmbedRequest) (embedding.EmbedResponse, error) {
				e := embedding.Embedding{Vector: []float32{1, 0}, Dimensions: 2, Index: 0}
				return embedding.EmbedResponse{Embeddings: []embedding.Embedding{e}}, nil
			},
		},
		{
			name: "duplicate_index",
			responder: func(_ context.Context, req embedding.EmbedRequest) (embedding.EmbedResponse, error) {
				embs := make([]embedding.Embedding, len(req.Inputs))
				for i := range req.Inputs {
					embs[i] = embedding.Embedding{Vector: []float32{1, 0}, Dimensions: 2, Index: 0}
				}
				return embedding.EmbedResponse{Embeddings: embs}, nil
			},
		},
		{
			name: "out_of_range_index",
			responder: func(_ context.Context, req embedding.EmbedRequest) (embedding.EmbedResponse, error) {
				embs := make([]embedding.Embedding, len(req.Inputs))
				for i := range req.Inputs {
					embs[i] = embedding.Embedding{Vector: []float32{1, 0}, Dimensions: 2, Index: i + 100}
				}
				return embedding.EmbedResponse{Embeddings: embs}, nil
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newSemantic(t, embedtestutil.NewFakeProvider(embedtestutil.WithResponder(tc.responder)))
			_, err := m.Compute(context.Background(), semanticScored([2]string{"a", "b"}))
			if err == nil {
				t.Fatal("expected malformed-response error, got nil")
			}
			var appErr *apperrors.AppError
			if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeExternalService {
				t.Errorf("error = %v, want AppError %s", err, apperrors.ErrCodeExternalService)
			}
		})
	}
}

func TestSemanticSimilarityRejectsNonFiniteVector(t *testing.T) {
	t.Parallel()

	// An untrusted provider may return a NaN/Inf component; it must surface as a
	// typed external-service failure, never a non-finite score that corrupts
	// aggregation and cannot be persisted as JSON.
	provider := embedtestutil.NewFakeProvider(
		embedtestutil.WithVector("pred", []float32{float32(math.NaN()), 0}),
		embedtestutil.WithVector("ref", []float32{1, 0}),
	)
	m := newSemantic(t, provider)
	_, err := m.Compute(context.Background(), semanticScored([2]string{"pred", "ref"}))
	if err == nil {
		t.Fatal("expected non-finite error, got nil")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeExternalService {
		t.Errorf("error = %v, want AppError %s", err, apperrors.ErrCodeExternalService)
	}
}

func TestSemanticSimilarityNonPositiveTimeoutStaysBounded(t *testing.T) {
	t.Parallel()

	// A non-positive WithSemanticTimeout must not clear an existing bound: it is
	// ignored, so a stalled provider still times out rather than hanging the run.
	policy := resilience.NewPolicy().WithTimeout(10 * time.Millisecond)
	provider := embedtestutil.NewFakeProvider(embedtestutil.WithBlockUntilCancel())
	m := newSemantic(t, provider,
		metric.WithSemanticPolicy[string](policy), metric.WithSemanticTimeout[string](0))
	_, err := m.Compute(context.Background(), semanticScored([2]string{"a", "b"}))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded (non-positive timeout must not clear the bound)", err)
	}
}

func TestSemanticSimilarityRejectsZeroValueEmbedding(t *testing.T) {
	t.Parallel()

	// A provider may preallocate a zero-value embedding (empty vector,
	// Dimensions 0) for a missing response item. Accepting it would score a
	// spurious similarity, so it must surface as a typed external-service failure.
	responder := func(_ context.Context, req embedding.EmbedRequest) (embedding.EmbedResponse, error) {
		embs := make([]embedding.Embedding, len(req.Inputs))
		for i := range req.Inputs {
			embs[i] = embedding.Embedding{Index: i} // zero-value: no vector, Dimensions 0
		}
		return embedding.EmbedResponse{Embeddings: embs}, nil
	}
	m := newSemantic(t, embedtestutil.NewFakeProvider(embedtestutil.WithResponder(responder)))
	_, err := m.Compute(context.Background(), semanticScored([2]string{"a", "b"}))
	if err == nil {
		t.Fatal("expected zero-value-embedding error, got nil")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeExternalService {
		t.Errorf("error = %v, want AppError %s", err, apperrors.ErrCodeExternalService)
	}
}

func TestSemanticSimilarityAppliesFreshTimeoutPerCall(t *testing.T) {
	t.Parallel()

	// Each batch is embedded under its own fresh deadline, not one dataset-wide
	// budget. With batch size 1 and a run context that carries no deadline, every
	// provider call must still receive a deadline close to the configured timeout.
	const timeout = 30 * time.Second
	responder := func(ctx context.Context, req embedding.EmbedRequest) (embedding.EmbedResponse, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			return embedding.EmbedResponse{}, errors.New("call context has no deadline")
		}
		if remaining := time.Until(deadline); remaining < timeout/2 {
			return embedding.EmbedResponse{}, errors.New("call context deadline is not fresh")
		}
		embs := make([]embedding.Embedding, len(req.Inputs))
		for i := range req.Inputs {
			embs[i] = embedding.Embedding{Vector: []float32{1, 0}, Dimensions: 2, Index: i}
		}
		return embedding.EmbedResponse{Embeddings: embs}, nil
	}
	m := newSemantic(t, embedtestutil.NewFakeProvider(embedtestutil.WithResponder(responder)),
		metric.WithSemanticTimeout[string](timeout), metric.WithSemanticBatchSize[string](1))
	if _, err := m.Compute(context.Background(), semanticScored(
		[2]string{"a", "a"}, [2]string{"b", "b"},
	)); err != nil {
		t.Fatalf("Compute: %v", err)
	}
}

func TestSemanticSimilarityPolicyRetriesTransientFailures(t *testing.T) {
	t.Parallel()

	// Provider calls route through the injected resilience policy: a transient
	// failure is retried under a bounded-attempt policy, and a later success is
	// scored. This proves the canonical resilience seam governs each call.
	var mu sync.Mutex
	attempts := 0
	responder := func(ctx context.Context, req embedding.EmbedRequest) (embedding.EmbedResponse, error) {
		mu.Lock()
		attempts++
		fail := attempts <= 2
		mu.Unlock()
		if fail {
			return embedding.EmbedResponse{}, errors.New("transient")
		}
		embs := make([]embedding.Embedding, len(req.Inputs))
		for i := range req.Inputs {
			embs[i] = embedding.Embedding{Vector: []float32{1, 0}, Dimensions: 2, Index: i}
		}
		return embedding.EmbedResponse{Embeddings: embs}, nil
	}
	policy := resilience.NewPolicy().WithRetry(resilience.RetryConfig{
		MaxAttempts: 3,
		RetryIf:     func(error) bool { return true },
	})
	m := newSemantic(t, embedtestutil.NewFakeProvider(embedtestutil.WithResponder(responder)),
		metric.WithSemanticPolicy[string](policy))
	if _, err := m.Compute(context.Background(), semanticScored([2]string{"a", "a"})); err != nil {
		t.Fatalf("Compute after retries: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3 (two failures then success)", attempts)
	}
}

func TestSemanticSimilarityWithPolicyEnforcesTimeout(t *testing.T) {
	t.Parallel()

	provider := embedtestutil.NewFakeProvider(embedtestutil.WithBlockUntilCancel())
	policy := resilience.NewPolicy().WithTimeout(10 * time.Millisecond)
	m := newSemantic(t, provider, metric.WithSemanticPolicy[string](policy))
	_, err := m.Compute(context.Background(), semanticScored([2]string{"a", "b"}))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeTimeout {
		t.Errorf("error = %v, want AppError %s", err, apperrors.ErrCodeTimeout)
	}
}
