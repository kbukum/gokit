package httpclient_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/security"
)

type loopbackDNS struct{ calls atomic.Int32 }

func (d *loopbackDNS) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	d.calls.Add(1)
	return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
}

func loopbackPolicy(t *testing.T, origin string, dns security.EndpointResolver) *security.EndpointPolicy {
	t.Helper()
	policy, err := security.NewEndpointPolicy([]security.EndpointRule{
		{Origin: origin, AllowHTTP: true, PrivateRanges: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}},
	}, dns, &net.Dialer{})
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func TestEndpointPolicySendsStructuredQueryAndResolvesOnlyAtDial(t *testing.T) {
	t.Parallel()
	queries := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries <- r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	origin := strings.Replace(server.URL, "127.0.0.1", "model.localhost", 1)
	dns := &loopbackDNS{}
	client, err := httpclient.New(httpclient.Config{BaseURL: origin, EndpointPolicy: loopbackPolicy(t, origin, dns)})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Unwrap().CloseIdleConnections()
	for range 2 {
		if _, err := client.Do(t.Context(), httpclient.Request{Method: http.MethodGet, Path: "/v1/models", Query: map[string]string{"api-version": "2024-10-21"}}); err != nil {
			t.Fatal(err)
		}
		if got := <-queries; got != "api-version=2024-10-21" {
			t.Fatalf("query = %q", got)
		}
	}
	if calls := dns.calls.Load(); calls != 1 {
		t.Fatalf("DNS lookups = %d, want one per connection", calls)
	}
}

func TestEndpointPolicyErrorsOmitQueryValues(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.NotFoundHandler())
	origin := server.URL
	server.Close()
	client, err := httpclient.New(httpclient.Config{BaseURL: origin, EndpointPolicy: loopbackPolicy(t, origin, &loopbackDNS{})})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Unwrap().CloseIdleConnections()
	const secret = "query-sentinel"
	_, err = client.Do(t.Context(), httpclient.Request{Method: http.MethodGet, Path: "/v1", Query: map[string]string{"q": secret}})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unreachable endpoint error = %v", err)
	}
}
