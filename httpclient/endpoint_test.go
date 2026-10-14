package httpclient_test

import (
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/security"
)

func TestEndpointPolicyRejectsUnsafeClientAndURLsWithoutSecretEcho(t *testing.T) {
	t.Parallel()
	policy, err := security.NewEndpointPolicy([]security.EndpointRule{{Origin: "https://model.example"}}, net.DefaultResolver, &net.Dialer{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tlsCfg := range []*security.TLSConfig{
		{MinVersion: tls.VersionTLS12}, {ServerName: "unchecked.example"},
	} {
		if client, err := httpclient.New(httpclient.Config{EndpointPolicy: policy, TLS: tlsCfg}); client != nil || err == nil {
			t.Fatal("endpoint client weakened pinned TLS policy")
		}
	}
	if client, err := httpclient.New(httpclient.Config{EndpointPolicy: policy, ResiliencePolicy: resilience.NewPolicy().WithRetry(*httpclient.DefaultRetryConfig())}); client != nil || err == nil {
		t.Fatal("implicit provider retries accepted")
	}
	client, err := httpclient.New(httpclient.Config{EndpointPolicy: policy})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Unwrap().CloseIdleConnections()
	const secret = "credential-sentinel"
	for _, path := range []string{
		"https://user:" + secret + "@model.example/v1", "https://model.example/?key=" + secret,
		"https://[" + secret + "/model", "https://model.example/#" + secret,
	} {
		response, err := client.Do(t.Context(), httpclient.Request{Method: http.MethodPost, Path: path})
		if response != nil || err == nil || strings.Contains(err.Error(), secret) {
			t.Fatal("invalid endpoint accepted or echoed credential content")
		}
	}
}
