package security_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"testing"

	"github.com/kbukum/gokit/security"
)

type endpointDNS struct {
	addresses []netip.Addr
	err       error
}

func (r *endpointDNS) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r.addresses, r.err
}

type endpointDial struct {
	calls   int
	address string
	conn    net.Conn
	err     error
}

func (d *endpointDial) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	d.calls++
	d.address = address
	if d.conn != nil || d.err != nil {
		return d.conn, d.err
	}
	return nil, errors.New("dial sentinel")
}

type emptyEndpointDial struct{}

func (emptyEndpointDial) DialContext(context.Context, string, string) (net.Conn, error) {
	return nil, nil //nolint:nilnil // Exercise a dependency violating the successful-dial contract.
}

func TestEndpointPolicyOwnsFailedDialConnections(t *testing.T) {
	t.Parallel()
	dns := &endpointDNS{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}}
	local, remote := net.Pipe()
	t.Cleanup(func() {
		if err := remote.Close(); err != nil {
			t.Error(err)
		}
	})
	outage := errors.New("connection setup failed")
	policy, err := security.NewEndpointPolicy([]security.EndpointRule{{Origin: "https://model.example"}}, dns, &endpointDial{conn: local, err: outage})
	if err != nil {
		t.Fatal(err)
	}
	if conn, err := policy.DialContext(t.Context(), "tcp", "model.example:443"); conn != nil || !errors.Is(err, outage) {
		t.Fatal("failed dial leaked a connection or lost its cause")
	}
	if _, err := remote.Write([]byte("x")); !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("failed dial connection was not closed")
	}
	empty, err := security.NewEndpointPolicy([]security.EndpointRule{{Origin: "https://model.example"}}, dns, emptyEndpointDial{})
	if err != nil {
		t.Fatal(err)
	}
	if conn, err := empty.DialContext(t.Context(), "tcp", "model.example:443"); conn != nil || err == nil {
		t.Fatal("missing connection reported successful dial")
	}
}

func TestEndpointPolicyChecksEveryDNSAnswerAtDial(t *testing.T) {
	t.Parallel()
	dns := &endpointDNS{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}}
	dial := &endpointDial{}
	policy, err := security.NewEndpointPolicy([]security.EndpointRule{{Origin: "https://model.example:443"}}, dns, dial)
	if err != nil {
		t.Fatal(err)
	}
	if url, err := policy.Validate(t.Context(), "https://MODEL.example/v1/chat"); err != nil || url != "https://model.example:443/v1/chat" {
		t.Fatalf("approved endpoint normalization failed: %v", err)
	}
	if _, err := policy.DialContext(t.Context(), "tcp", "model.example:443"); err == nil || dial.calls != 1 || dial.address != "8.8.8.8:443" {
		t.Fatal("dial did not use the validated numeric address")
	}
	dns.addresses = []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")}
	if _, err := policy.DialContext(t.Context(), "tcp", "model.example:443"); err == nil || dial.calls != 1 {
		t.Fatal("mixed public/private DNS answers bypassed address policy")
	}
	dns.addresses = []netip.Addr{netip.MustParseAddr("169.254.169.254")}
	if _, err := policy.Validate(t.Context(), "https://model.example/v1/chat"); err == nil {
		t.Fatal("DNS rebinding bypassed pinned policy")
	}
	for _, address := range []string{"other.example:443", "model.example:80", "model.example", "model.example:0"} {
		if _, err := policy.DialContext(t.Context(), "tcp", address); err == nil || dial.calls != 1 {
			t.Fatal("unchecked authority reached the dialer")
		}
	}
	dns.err = errors.New("DNS outage")
	if _, err := policy.Validate(t.Context(), "https://model.example/v1/chat"); !errors.Is(err, dns.err) {
		t.Fatal("DNS outage cause was not preserved")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := policy.DialContext(ctx, "tcp", "model.example:443"); !errors.Is(err, context.Canceled) {
		t.Fatal("dial cancellation was not preserved")
	}
}

func TestEndpointPolicyRequiresExplicitPrivateRangesAndUnambiguousURLs(t *testing.T) {
	t.Parallel()
	dns, dial := &endpointDNS{}, &endpointDial{}
	rules := []security.EndpointRule{{Origin: "http://127.0.0.1:8080", AllowHTTP: true, PrivateRanges: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}}}
	policy, err := security.NewEndpointPolicy(rules, dns, dial)
	if err != nil {
		t.Fatal(err)
	}
	rules[0].PrivateRanges[0] = netip.MustParsePrefix("10.0.0.0/8")
	if _, err := policy.Validate(t.Context(), "http://127.0.0.1:8080/model"); err != nil {
		t.Fatal("explicit private range was not copied or allowed")
	}
	for _, raw := range []string{
		"http://user:secret@127.0.0.1:8080/model", "http://127.0.0.1:8080/?key=secret",
		"http://127.0.0.1:8080/?", "http://127.0.0.1:8080/#", "http://127.0.0.1:8080/path%2Fmodel",
		"https://model.example./", "http://127.0.0.1:8081/model", "ftp://127.0.0.1:8080/model", "http://127.0.0.1:65536/model",
	} {
		if _, err := policy.Validate(t.Context(), raw); err == nil {
			t.Fatal("unsafe or unchecked URL accepted")
		}
	}
	for _, rule := range []security.EndpointRule{
		{Origin: "http://model.example"},
		{Origin: "https://model.example/path"},
		{Origin: "https://model.example", PrivateRanges: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}},
		{Origin: "https://model.example", PrivateRanges: []netip.Prefix{{}}},
	} {
		if p, err := security.NewEndpointPolicy([]security.EndpointRule{rule}, dns, dial); p != nil || err == nil {
			t.Fatal("unbounded or unsafe operator rule accepted")
		}
	}
}
