//go:build integration

package httpclient_test

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/security/tlstest"
)

type policyDNS struct{ calls atomic.Int32 }

func (d *policyDNS) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	d.calls.Add(1)
	return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
}

func TestEndpointPolicyRefusesRedirectBeforeCredentialRedispatch(t *testing.T) {
	t.Parallel()
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	policy, err := security.NewEndpointPolicy([]security.EndpointRule{
		{Origin: server.URL, AllowHTTP: true, PrivateRanges: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}},
	}, net.DefaultResolver, &net.Dialer{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	client, err := httpclient.New(httpclient.Config{BaseURL: server.URL, EndpointPolicy: policy, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Unwrap().CloseIdleConnections()
	if _, err := client.Do(t.Context(), httpclient.Request{Method: http.MethodPost, Path: "/start", Auth: httpclient.BearerAuth("provider-sentinel")}); err == nil {
		t.Fatal("endpoint policy followed a credential-bearing redirect")
	}
	if received.Load() != 1 {
		t.Fatal("redirect caused another provider dispatch")
	}
}

func TestEndpointPolicyDialsValidatedAddressAndPreservesTLSHostname(t *testing.T) {
	t.Parallel()
	certs := tlstest.GenerateTLSCerts(t, tlstest.WithHosts("model.example"))
	var serverName atomic.Value
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS.Version != tls.VersionTLS13 || r.Header.Get("Authorization") != "Bearer provider-sentinel" {
			t.Error("provider connection lost secure header/TLS contract")
		}
		serverName.Store(r.TLS.ServerName)
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certs.ServerTLS}, MinVersion: tls.VersionTLS13}
	server.StartTLS()
	defer server.Close()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "https://"))
	if err != nil {
		t.Fatal(err)
	}
	origin := "https://model.example:" + port
	dns := &policyDNS{}
	policy, err := security.NewEndpointPolicy([]security.EndpointRule{
		{Origin: origin, PrivateRanges: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}},
	}, dns, &net.Dialer{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	client, err := httpclient.New(httpclient.Config{
		BaseURL: origin, EndpointPolicy: policy, Timeout: 2 * time.Second,
		TLS: &security.TLSConfig{CAFile: certs.CAFile, MinVersion: tls.VersionTLS13},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Unwrap().CloseIdleConnections()
	if response, err := client.Do(t.Context(), httpclient.Request{Method: http.MethodPost, Path: "/model", Auth: httpclient.BearerAuth("provider-sentinel")}); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("validated provider connection failed: %v", err)
	}
	if serverName.Load() != "model.example" || dns.calls.Load() < 2 {
		t.Fatal("numeric dialing bypassed hostname verification/SNI or connection-time DNS checks")
	}
	if _, err := client.Do(t.Context(), httpclient.Request{Method: http.MethodGet, Path: "/model", Query: map[string]string{"secret": "must-not-dispatch"}}); err == nil {
		t.Fatal("query credentials bypassed request policy")
	}
}
