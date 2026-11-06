package fetchtest

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/security"
)

// Adapter returns an adapter with a five-second timeout and optional default auth, whose endpoint policy grants
// each origin over HTTP on 127.0.0.1 only.
func Adapter(t testing.TB, auth *httpclient.AuthConfig, origins ...string) *httpclient.Adapter {
	t.Helper()
	loopback := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	rules := make([]security.EndpointRule, 0, len(origins))
	for _, o := range origins {
		rules = append(rules, security.EndpointRule{Origin: o, AllowHTTP: true, PrivateRanges: loopback})
	}
	policy, err := security.NewEndpointPolicy(rules, net.DefaultResolver, &net.Dialer{})
	if err != nil {
		t.Fatalf("endpoint policy: %v", err)
	}
	hc, err := httpclient.New(httpclient.Config{Timeout: 5 * time.Second, Auth: auth, EndpointPolicy: policy})
	if err != nil {
		// Not the error itself: adapter errors may carry configured file paths, which stay out of test logs.
		t.Fatal("http adapter: the test adapter configuration was refused")
	}
	return hc
}
