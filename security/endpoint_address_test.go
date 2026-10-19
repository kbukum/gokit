package security_test

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/kbukum/gokit/security"
)

type sequenceDial struct {
	tried []string
	fail  map[string]error
}

func (d *sequenceDial) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	d.tried = append(d.tried, address)
	if err := d.fail[address]; err != nil {
		return nil, err
	}
	local, remote := net.Pipe()
	_ = remote.Close()
	return local, nil
}

func TestEndpointPolicyFallsBackAcrossResolvedAddresses(t *testing.T) {
	t.Parallel()
	refused := errors.New("refused")
	dns := &endpointDNS{addresses: []netip.Addr{netip.MustParseAddr("2606:4700::1"), netip.MustParseAddr("8.8.8.8")}}
	dial := &sequenceDial{fail: map[string]error{"[2606:4700::1]:443": refused}}
	policy, err := security.NewEndpointPolicy([]security.EndpointRule{{Origin: "https://model.example"}}, dns, dial)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := policy.DialContext(t.Context(), "tcp", "model.example:443")
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := conn.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if len(dial.tried) != 2 || dial.tried[1] != "8.8.8.8:443" {
		t.Fatalf("dial attempts = %v", dial.tried)
	}
	dial.fail["8.8.8.8:443"] = refused
	if _, err := policy.DialContext(t.Context(), "tcp", "model.example:443"); !errors.Is(err, refused) {
		t.Fatalf("all-address failure lost its cause: %v", err)
	}
}

func TestEndpointPolicyDialBudgetOption(t *testing.T) {
	t.Parallel()
	rules := []security.EndpointRule{{Origin: "https://model.example"}}
	for _, budget := range []time.Duration{0, -time.Second, security.MaxEndpointDialBudget + 1} {
		if _, err := security.NewEndpointPolicy(rules, &endpointDNS{}, &endpointDial{}, security.WithEndpointDialBudget(budget)); err == nil {
			t.Fatalf("budget %v accepted", budget)
		}
	}
	if _, err := security.NewEndpointPolicy(rules, &endpointDNS{}, &endpointDial{}, nil); err == nil {
		t.Fatal("nil option accepted")
	}
	if _, err := security.NewEndpointPolicy(rules, &endpointDNS{}, &endpointDial{}, security.WithEndpointDialBudget(30*time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestEndpointPolicyClassifiesSpecialPurposeAddresses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		address string
		grant   string
		allowed bool
	}{
		{address: "8.8.8.8", allowed: true},
		{address: "64:ff9b::808:808", allowed: true},
		{address: "2002:808:808::1", allowed: true},
		{address: "100.100.100.200"},
		{address: "100.100.100.200", grant: "100.64.0.0/10", allowed: true},
		{address: "198.18.0.1"},
		{address: "192.0.0.170"},
		{address: "64:ff9b::a9fe:a9fe"},
		{address: "64:ff9b::a00:1"},
		{address: "2002:a00:1::1"},
		{address: "64:ff9b:1::1"},
		{address: "::a00:1"},
		{address: "0.1.2.3"},
		{address: "240.0.0.1"},
		{address: "255.255.255.255"},
		{address: "192.0.2.1"},
		{address: "2001:db8::1"},
		{address: "2001::1"},
		{address: "::ffff:169.254.169.254"},
		{address: "::1", grant: "::1/128", allowed: true},
	} {
		rule := security.EndpointRule{Origin: "https://model.example"}
		if tc.grant != "" {
			rule.PrivateRanges = []netip.Prefix{netip.MustParsePrefix(tc.grant)}
		}
		dns := &endpointDNS{addresses: []netip.Addr{netip.MustParseAddr(tc.address)}}
		policy, err := security.NewEndpointPolicy([]security.EndpointRule{rule}, dns, &endpointDial{})
		if err != nil {
			t.Fatalf("%s: %v", tc.address, err)
		}
		_, err = policy.Validate(t.Context(), "https://model.example/v1")
		if (err == nil) != tc.allowed {
			t.Errorf("%s grant=%q: err=%v, want allowed=%v", tc.address, tc.grant, err, tc.allowed)
		}
	}
}

func FuzzNormalizeEndpointURL(f *testing.F) {
	for _, seed := range []string{"https://model.example/v1", "http://127.0.0.1:8080/", "https://[::1]:443/x", "https://user@h/", "https://h/?q#f", "https://h./", "https://xn--h/%2F"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		normalized, err := security.NormalizeEndpointURL(raw)
		if err != nil {
			return
		}
		again, err := security.NormalizeEndpointURL(normalized)
		if err != nil || again != normalized {
			t.Fatalf("normalization is not stable: %q -> %q -> %q (%v)", raw, normalized, again, err)
		}
	})
}
