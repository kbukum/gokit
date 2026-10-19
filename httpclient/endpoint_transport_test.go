package httpclient

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/kbukum/gokit/security"
)

func TestEndpointPolicyKeepsConfiguredRequestBudgets(t *testing.T) {
	t.Parallel()
	policy, err := security.NewEndpointPolicy([]security.EndpointRule{{Origin: "https://model.example"}}, net.DefaultResolver, &net.Dialer{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(Config{EndpointPolicy: policy, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Unwrap().CloseIdleConnections()
	guarded, ok := client.httpClient.Transport.(*endpointTransport)
	if !ok {
		t.Fatal("endpoint policy transport is not installed")
	}
	transport, ok := guarded.next.(*http.Transport)
	if !ok {
		t.Fatal("endpoint policy wraps an unexpected transport")
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatal("default transport has an unexpected type")
	}
	if transport.ResponseHeaderTimeout != 0 || transport.TLSHandshakeTimeout != base.TLSHandshakeTimeout || client.httpClient.Timeout != time.Minute {
		t.Fatalf("policy overrode request budgets: header=%v handshake=%v total=%v", transport.ResponseHeaderTimeout, transport.TLSHandshakeTimeout, client.httpClient.Timeout)
	}
}
