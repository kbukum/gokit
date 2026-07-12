package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPrometheusHandler(t *testing.T) {
	if PrometheusHandler() == nil {
		t.Fatal("expected non-nil prometheus handler")
	}
}

func TestRegisterPrometheusEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	RegisterPrometheusEndpoint(mux, "/metrics")

	req := httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from metrics endpoint, got %d", rec.Code)
	}
}
