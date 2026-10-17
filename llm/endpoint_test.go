package llm_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/llm/providers/openai"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/security"
)

type fixedResolver struct{ addr netip.Addr }

func (r fixedResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{r.addr}, nil
}

func endpointAdapter(t *testing.T, base string, ranges []netip.Prefix) *llm.Adapter {
	t.Helper()
	policy, err := security.NewEndpointPolicy([]security.EndpointRule{{Origin: "http://model.test:" + port(t, base), AllowHTTP: true, PrivateRanges: ranges}},
		fixedResolver{netip.MustParseAddr("127.0.0.1")}, &net.Dialer{})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := llm.NewWithDialect(&openai.Dialect{}, llm.Config{BaseURL: "http://model.test:" + port(t, base), Model: "m", EndpointPolicy: policy})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adapter.Close(context.Background()) })
	return adapter
}

func port(t *testing.T, raw string) string {
	t.Helper()
	_, p, err := net.SplitHostPort(raw[len("http://"):])
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func request() llm.CompletionRequest {
	return llm.CompletionRequest{Messages: []chat.Message{chat.User("hi")}}
}

func TestEndpointPolicyGuardsEveryConnection(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)

	allowed := endpointAdapter(t, srv.URL, []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	resp, err := allowed.Execute(t.Context(), request())
	if err != nil || resp.Text() != "ok" {
		t.Fatalf("granted loopback endpoint = %q, %v", resp.Text(), err)
	}

	refused := endpointAdapter(t, srv.URL, nil)
	if _, err := refused.Execute(t.Context(), request()); err == nil {
		t.Fatal("a private address outside the grant was dialed")
	}
	if _, err := refused.Stream(t.Context(), request()); err == nil {
		t.Fatal("a stream reached a private address outside the grant")
	}
	if calls != 1 {
		t.Fatalf("server saw %d calls, want only the granted one", calls)
	}
}

func TestEndpointPolicyRefusesRedirectsAndRetries(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	adapter := endpointAdapter(t, srv.URL, []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	if _, err := adapter.Execute(t.Context(), request()); err == nil {
		t.Fatal("redirect followed")
	}

	policy, err := security.NewEndpointPolicy([]security.EndpointRule{{Origin: "https://model.example"}}, net.DefaultResolver, &net.Dialer{})
	if err != nil {
		t.Fatal(err)
	}
	retry := resilience.NewPolicy().WithRetry(resilience.DefaultRetryConfig())
	if a, err := llm.NewWithDialect(&openai.Dialect{}, llm.Config{BaseURL: "https://model.example", EndpointPolicy: policy, ResiliencePolicy: retry}); a != nil || err == nil {
		t.Fatal("implicit retries accepted with an endpoint policy")
	}
}
