package endpoint_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/server/endpoint"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newRouter constructs a router and mounts the handler at the given path.
func newRouter(path string, h gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.GET(path, h)
	return r
}

// do executes a GET request against the router and returns the recorder + decoded JSON body.
func do(t *testing.T, r *gin.Engine, path string) (rec *httptest.ResponseRecorder, body map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v\nbody=%s", err, w.Body.String())
		}
	}
	return w, body
}

// ─── Health ────────────────────────────────────────────────────────────────

func TestHealth_NilChecker_ReturnsHealthy(t *testing.T) {
	r := newRouter("/health", endpoint.Health("svc", nil))

	w, body := do(t, r, "/health")

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if body["status"] != "healthy" {
		t.Errorf("status: got %v want healthy", body["status"])
	}
	if body["service"] != "svc" {
		t.Errorf("service: got %v want svc", body["service"])
	}
	if _, ok := body["timestamp"].(string); !ok {
		t.Errorf("timestamp missing or wrong type: %v", body["timestamp"])
	}
	// Components is nil from a nil checker — JSON renders as null.
	if body["components"] != nil {
		t.Errorf("components: got %v want nil", body["components"])
	}
}

func TestHealth_AllHealthy(t *testing.T) {
	checker := func(ctx context.Context) []component.Health {
		return []component.Health{
			{Name: "db", Status: component.StatusHealthy},
			{Name: "cache", Status: component.StatusHealthy},
		}
	}
	r := newRouter("/health", endpoint.Health("svc", checker))

	w, body := do(t, r, "/health")

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if body["status"] != "healthy" {
		t.Errorf("status: got %v want healthy", body["status"])
	}
	comps, _ := body["components"].([]any)
	if len(comps) != 2 {
		t.Fatalf("components: got %d want 2", len(comps))
	}
}

func TestHealth_DegradedComponent_ReturnsDegradedWith200(t *testing.T) {
	checker := func(ctx context.Context) []component.Health {
		return []component.Health{
			{Name: "db", Status: component.StatusHealthy},
			{Name: "cache", Status: component.StatusDegraded, Message: "slow"},
		}
	}
	r := newRouter("/health", endpoint.Health("svc", checker))

	w, body := do(t, r, "/health")

	if w.Code != http.StatusOK {
		t.Fatalf("degraded should still respond 200 (only unhealthy maps to 503); got %d", w.Code)
	}
	if body["status"] != "degraded" {
		t.Errorf("status: got %v want degraded", body["status"])
	}
}

func TestHealth_UnhealthyComponent_Returns503(t *testing.T) {
	checker := func(ctx context.Context) []component.Health {
		return []component.Health{
			{Name: "db", Status: component.StatusUnhealthy, Message: "conn refused"},
			{Name: "cache", Status: component.StatusHealthy},
		}
	}
	r := newRouter("/health", endpoint.Health("svc", checker))

	w, body := do(t, r, "/health")

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("unhealthy must return 503; got %d", w.Code)
	}
	if body["status"] != "unhealthy" {
		t.Errorf("status: got %v want unhealthy", body["status"])
	}
}

// Ensures unhealthy "wins" over a degraded component regardless of order.
func TestHealth_UnhealthyOverridesDegraded(t *testing.T) {
	checker := func(ctx context.Context) []component.Health {
		return []component.Health{
			{Name: "cache", Status: component.StatusDegraded},
			{Name: "db", Status: component.StatusUnhealthy},
		}
	}
	r := newRouter("/health", endpoint.Health("svc", checker))

	w, body := do(t, r, "/health")

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status code: got %d want 503", w.Code)
	}
	if body["status"] != "unhealthy" {
		t.Errorf("status: got %v want unhealthy (must override degraded)", body["status"])
	}
}

// Verifies the request context flows into the checker.
func TestHealth_PassesRequestContext(t *testing.T) {
	type ctxKey string
	const key ctxKey = "k"

	var got any
	checker := func(ctx context.Context) []component.Health {
		got = ctx.Value(key)
		return nil
	}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), key, "v"))
		c.Next()
	})
	r.GET("/health", endpoint.Health("svc", checker))

	req := httptest.NewRequest(http.MethodGet, "/health", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got != "v" {
		t.Fatalf("checker did not receive request context; got %v", got)
	}
}

// ─── Liveness ──────────────────────────────────────────────────────────────

func TestLiveness_AlwaysReturnsAlive(t *testing.T) {
	r := newRouter("/alive", endpoint.Liveness("svc"))

	w, body := do(t, r, "/alive")

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if body["status"] != "alive" {
		t.Errorf("status: got %v want alive", body["status"])
	}
	if body["service"] != "svc" {
		t.Errorf("service: got %v want svc", body["service"])
	}
	if _, err := time.Parse(time.RFC3339, body["timestamp"].(string)); err != nil {
		t.Errorf("timestamp not RFC3339: %v", err)
	}
}

// ─── Readiness ─────────────────────────────────────────────────────────────

func TestReadiness_NilChecker_Ready(t *testing.T) {
	r := newRouter("/ready", endpoint.Readiness("svc", nil))

	w, body := do(t, r, "/ready")

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if body["status"] != "ready" {
		t.Errorf("status: got %v want ready", body["status"])
	}
}

func TestReadiness_AllHealthy_Ready(t *testing.T) {
	checker := func(ctx context.Context) []component.Health {
		return []component.Health{{Name: "db", Status: component.StatusHealthy}}
	}
	r := newRouter("/ready", endpoint.Readiness("svc", checker))

	w, body := do(t, r, "/ready")

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if body["status"] != "ready" {
		t.Errorf("status: got %v want ready", body["status"])
	}
}

func TestReadiness_Unhealthy_NotReady503(t *testing.T) {
	checker := func(ctx context.Context) []component.Health {
		return []component.Health{
			{Name: "db", Status: component.StatusHealthy},
			{Name: "cache", Status: component.StatusUnhealthy},
		}
	}
	r := newRouter("/ready", endpoint.Readiness("svc", checker))

	w, body := do(t, r, "/ready")

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want 503", w.Code)
	}
	if body["status"] != "not_ready" {
		t.Errorf("status: got %v want not_ready", body["status"])
	}
}

// Degraded should still be considered ready (only unhealthy gates traffic).
func TestReadiness_Degraded_StillReady(t *testing.T) {
	checker := func(ctx context.Context) []component.Health {
		return []component.Health{{Name: "cache", Status: component.StatusDegraded}}
	}
	r := newRouter("/ready", endpoint.Readiness("svc", checker))

	w, body := do(t, r, "/ready")

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if body["status"] != "ready" {
		t.Errorf("status: got %v want ready", body["status"])
	}
}

// ─── Info ──────────────────────────────────────────────────────────────────

func TestInfo_ReturnsExpectedShape(t *testing.T) {
	r := newRouter("/info", endpoint.Info("svc"))

	w, body := do(t, r, "/info")

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if body["service"] != "svc" {
		t.Errorf("service: got %v want svc", body["service"])
	}
	for _, k := range []string{"version", "git_commit", "git_branch", "build_time", "go_version", "is_release", "is_dirty", "uptime", "timestamp"} {
		if _, ok := body[k]; !ok {
			t.Errorf("missing field %q", k)
		}
	}
	if _, ok := body["is_release"].(bool); !ok {
		t.Errorf("is_release should be bool, got %T", body["is_release"])
	}
	if !strings.HasPrefix(body["go_version"].(string), "go") {
		t.Errorf("go_version should start with 'go': %v", body["go_version"])
	}
	if _, err := time.Parse(time.RFC3339, body["timestamp"].(string)); err != nil {
		t.Errorf("timestamp not RFC3339: %v", err)
	}
}

// ─── Version ───────────────────────────────────────────────────────────────

func TestVersion_ReturnsExpectedShape(t *testing.T) {
	r := newRouter("/version", endpoint.Version())

	w, body := do(t, r, "/version")

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	for _, k := range []string{"version", "git_commit", "git_branch", "build_time", "go_version", "is_release", "is_dirty"} {
		if _, ok := body[k]; !ok {
			t.Errorf("missing field %q", k)
		}
	}
	// Should NOT include service-only fields.
	if _, present := body["service"]; present {
		t.Errorf("version endpoint should not include 'service'")
	}
	if _, present := body["uptime"]; present {
		t.Errorf("version endpoint should not include 'uptime'")
	}
}

// ─── Metrics ───────────────────────────────────────────────────────────────

func TestMetrics_ReturnsRuntimeShape(t *testing.T) {
	r := newRouter("/metrics", endpoint.Metrics())

	w, body := do(t, r, "/metrics")

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if _, err := time.Parse(time.RFC3339, body["timestamp"].(string)); err != nil {
		t.Errorf("timestamp not RFC3339: %v", err)
	}

	// goroutines is decoded as a JSON number → float64.
	g, ok := body["goroutines"].(float64)
	if !ok {
		t.Fatalf("goroutines: wrong type %T", body["goroutines"])
	}
	if g < 1 {
		t.Errorf("goroutines should be >=1, got %v", g)
	}

	mem, ok := body["memory"].(map[string]any)
	if !ok {
		t.Fatalf("memory: wrong type %T", body["memory"])
	}
	for _, k := range []string{"alloc_mb", "total_alloc_mb", "sys_mb", "gc_runs"} {
		if _, ok := mem[k].(float64); !ok {
			t.Errorf("memory.%s: missing or wrong type (%T)", k, mem[k])
		}
	}
}
