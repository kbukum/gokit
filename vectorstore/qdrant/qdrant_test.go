package qdrant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/kbukum/gokit/vectorstore"
)

type recordedRequest struct {
	Method, Path, APIKey string
	Body                 map[string]any
}

func newQdrantTestServer(t *testing.T, statuses []int, bodies []string) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []recordedRequest
	idx := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		rec := recordedRequest{Method: r.Method, Path: r.URL.RequestURI(), APIKey: r.Header.Get("api-key")}
		if r.Body != http.NoBody {
			_ = json.NewDecoder(r.Body).Decode(&rec.Body)
		}
		mu.Lock()
		seen = append(seen, rec)
		current := idx
		idx++
		mu.Unlock()
		status := http.StatusOK
		if current < len(statuses) {
			status = statuses[current]
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		body := "{}"
		if current < len(bodies) {
			body = bodies[current]
		}
		_, _ = w.Write([]byte(body))
	}))
	return server, &seen
}

func TestEnsureCollectionCreatesMissingCollection(t *testing.T) {
	t.Parallel()
	server, seen := newQdrantTestServer(t, []int{http.StatusNotFound, http.StatusOK}, []string{"{}", "{}"})
	defer server.Close()
	store, err := NewStore(Config{URL: server.URL, Metric: vectorstore.MetricDot})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.EnsureCollection(context.Background(), "tenant_vectors", 3); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}
	if (*seen)[1].Method != http.MethodPut || (*seen)[1].Path != "/collections/tenant_vectors" {
		t.Fatalf("create request = %#v", (*seen)[1])
	}
	vectors := (*seen)[1].Body["vectors"].(map[string]any)
	if vectors["distance"] != "Dot" || vectors["size"] != float64(3) {
		t.Fatalf("vectors body = %#v", vectors)
	}
}

func TestStoreMethodsRoundTripAgainstHTTP(t *testing.T) {
	t.Parallel()
	searchBody := `{"result":[{"id":42,"score":0.98,"payload":{"tag":"blue","count":7}}]}`
	server, seen := newQdrantTestServer(t, []int{200, 200, 200}, []string{"{}", searchBody, "{}"})
	defer server.Close()
	store, err := NewStore(Config{URL: server.URL, APIKey: "secret", Metric: vectorstore.MetricL2})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	ctx := context.Background()
	if err := store.Upsert(ctx, "tenant_vectors", vectorstore.Point{ID: "42", Vector: []float32{0.1, 0.2}, Payload: vectorstore.NewPointPayload().WithField("tag", "blue")}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	results, err := store.Search(ctx, "tenant_vectors", vectorstore.SearchQuery{Vector: []float32{0.1, 0.2}, Limit: 1, Filter: vectorstore.NewSearchFilter().MustMatch("tag", "blue")})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if err := store.Delete(ctx, "tenant_vectors", "42"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(results) != 1 || results[0].ID != "42" || results[0].Score != 0.98 {
		t.Fatalf("results = %#v", results)
	}
	for _, req := range *seen {
		if req.APIKey != "secret" {
			t.Fatalf("missing api key in %#v", req)
		}
	}
}

func TestRejectsUnsafeCollectionBeforeNetwork(t *testing.T) {
	t.Parallel()
	store, err := NewStore(Config{URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Delete(context.Background(), "../bad", "42"); err == nil || !strings.Contains(err.Error(), "collection") {
		t.Fatalf("expected collection error, got %v", err)
	}
}

func TestRegisterCapturesTypedConfig(t *testing.T) {
	t.Parallel()
	reg := vectorstore.NewFactoryRegistry()
	if err := Register(reg, Config{URL: "http://127.0.0.1:1"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	store, err := vectorstore.New(reg, vectorstore.Config{Provider: ProviderName, Metric: vectorstore.MetricDot})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if store == nil {
		t.Fatal("expected store")
	}
}

func TestHTTPStatusErrorsAndInvalidConfigPaths(t *testing.T) {
	t.Parallel()
	server, _ := newQdrantTestServer(t, []int{http.StatusInternalServerError}, []string{"boom"})
	defer server.Close()
	store, err := NewStore(Config{URL: server.URL})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.EnsureCollection(context.Background(), "tenant_vectors", 3); err == nil {
		t.Fatal("expected status error")
	}
	if _, err := NewStore(Config{URL: "https://qdrant.example.test", Metric: "bad"}); err == nil {
		t.Fatal("expected invalid metric")
	}
}

func TestSearchRejectsUnsupportedUpstreamPayload(t *testing.T) {
	t.Parallel()
	server, _ := newQdrantTestServer(t, []int{http.StatusOK}, []string{`{"result":[{"id":{"bad":true},"score":1,"payload":{}}]}`})
	defer server.Close()
	store, err := NewStore(Config{URL: server.URL})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := store.Search(context.Background(), "tenant_vectors", vectorstore.SearchQuery{Vector: []float32{1}, Limit: 1, Filter: nil}); err == nil {
		t.Fatal("expected bad id error")
	}
}

func TestStoreMethodsPropagateNetworkErrors(t *testing.T) {
	t.Parallel()
	store, err := NewStore(Config{URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	ctx := context.Background()
	if err := store.EnsureCollection(ctx, "tenant_vectors", 3); err == nil {
		t.Error("EnsureCollection should fail against unreachable server")
	}
	if err := store.Upsert(ctx, "tenant_vectors", vectorstore.Point{ID: "1", Vector: []float32{0.1}, Payload: nil}); err == nil {
		t.Error("Upsert should fail against unreachable server")
	}
	if _, err := store.Search(ctx, "tenant_vectors", vectorstore.SearchQuery{Vector: []float32{0.1}, Limit: 1, Filter: nil}); err == nil {
		t.Error("Search should fail against unreachable server")
	}
	if err := store.Delete(ctx, "tenant_vectors", "1"); err == nil {
		t.Error("Delete should fail against unreachable server")
	}
}

func TestUpsertRejectsInvalidPointID(t *testing.T) {
	t.Parallel()
	store, err := NewStore(Config{URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Upsert(context.Background(), "tenant_vectors", vectorstore.Point{ID: "not-a-uuid", Vector: []float32{0.1}, Payload: nil}); err == nil {
		t.Fatal("expected invalid point id error before network")
	}
	if err := store.Delete(context.Background(), "tenant_vectors", "not-a-uuid"); err == nil {
		t.Fatal("expected invalid point id error before network")
	}
}

func TestUpsertRejectsUnsupportedPayload(t *testing.T) {
	t.Parallel()
	store, err := NewStore(Config{URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	payload := vectorstore.NewPointPayload().WithField("nested", map[string]string{"bad": "value"})
	if err := store.Upsert(context.Background(), "tenant_vectors", vectorstore.Point{ID: "1", Vector: []float32{0.1}, Payload: payload}); err == nil {
		t.Fatal("expected unsupported payload error before network")
	}
}

func TestSearchRejectsUnsupportedFilterValue(t *testing.T) {
	t.Parallel()
	store, err := NewStore(Config{URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	filter := vectorstore.NewSearchFilter().MustMatch("nested", map[string]string{"bad": "value"})
	if _, err := store.Search(context.Background(), "tenant_vectors", vectorstore.SearchQuery{Vector: []float32{0.1}, Limit: 1, Filter: filter}); err == nil {
		t.Fatal("expected unsupported filter value error before network")
	}
}

func TestSearchRejectsUnsupportedFilterValueWithZeroLimit(t *testing.T) {
	t.Parallel()
	store, err := NewStore(Config{URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	filter := vectorstore.NewSearchFilter().MustMatch("nested", map[string]string{"bad": "value"})
	if _, err := store.Search(context.Background(), "tenant_vectors", vectorstore.SearchQuery{Vector: []float32{0.1}, Limit: 0, Filter: filter}); err == nil {
		t.Fatal("expected unsupported filter value error even when limit is zero")
	}
}

func TestSearchRejectsMalformedResponse(t *testing.T) {
	t.Parallel()
	server, _ := newQdrantTestServer(t, []int{http.StatusOK}, []string{`{"result":`})
	defer server.Close()
	store, err := NewStore(Config{URL: server.URL})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := store.Search(context.Background(), "tenant_vectors", vectorstore.SearchQuery{Vector: []float32{0.1}, Limit: 1, Filter: nil}); err == nil {
		t.Fatal("expected decode error for malformed response")
	}
}

func TestSearchRejectsUnsupportedReturnedPayload(t *testing.T) {
	t.Parallel()
	body := `{"result":[{"id":1,"score":1,"payload":{"nested":{"x":1}}}]}`
	server, _ := newQdrantTestServer(t, []int{http.StatusOK}, []string{body})
	defer server.Close()
	store, err := NewStore(Config{URL: server.URL})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := store.Search(context.Background(), "tenant_vectors", vectorstore.SearchQuery{Vector: []float32{0.1}, Limit: 1, Filter: nil}); err == nil {
		t.Fatal("expected unsupported returned payload error")
	}
}

func TestValidateCollectionRejectsDotSegments(t *testing.T) {
	t.Parallel()
	store, err := NewStore(Config{URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	for _, name := range []string{".", "..", ""} {
		if err := store.EnsureCollection(context.Background(), name, 3); err == nil {
			t.Fatalf("expected rejection of collection %q", name)
		}
	}
}

func TestRegisterUsesConfiguredMetricWhenCommonEmpty(t *testing.T) {
	t.Parallel()
	reg := vectorstore.NewFactoryRegistry()
	if err := Register(reg, Config{URL: "https://qdrant.example.test", Metric: vectorstore.MetricL2}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	factory, ok := reg.Get(ProviderName)
	if !ok {
		t.Fatal("qdrant provider missing")
	}
	store, err := factory(vectorstore.Config{Provider: ProviderName})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if store.(*Store).metric != vectorstore.MetricL2 {
		t.Fatalf("expected configured metric to be used, got %q", store.(*Store).metric)
	}
}

func TestRegisterRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	reg := vectorstore.NewFactoryRegistry()
	if err := Register(reg, Config{URL: "ftp://bad", Metric: vectorstore.MetricCosine}); err == nil {
		t.Fatal("expected invalid config rejection")
	}
}

func TestStoreMethodsReturnErrorOnUpstreamFailure(t *testing.T) {
	t.Parallel()
	server, _ := newQdrantTestServer(t,
		[]int{http.StatusInternalServerError, http.StatusInternalServerError, http.StatusInternalServerError},
		[]string{"boom", "boom", "boom"})
	defer server.Close()
	store, err := NewStore(Config{URL: server.URL})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	ctx := context.Background()
	if err := store.Upsert(ctx, "tenant_vectors", vectorstore.Point{ID: "1", Vector: []float32{0.1}, Payload: nil}); err == nil {
		t.Error("Upsert should surface upstream 500")
	}
	if _, err := store.Search(ctx, "tenant_vectors", vectorstore.SearchQuery{Vector: []float32{0.1}, Limit: 1, Filter: nil}); err == nil {
		t.Error("Search should surface upstream 500")
	}
	if err := store.Delete(ctx, "tenant_vectors", "1"); err == nil {
		t.Error("Delete should surface upstream 500")
	}
}

func TestRegisterRejectsNilRegistry(t *testing.T) {
	t.Parallel()
	if err := Register(nil, Config{URL: "http://localhost:6333"}); err == nil {
		t.Fatal("expected nil registry error")
	}
}
