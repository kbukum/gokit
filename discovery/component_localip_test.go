package discovery

import (
	"errors"
	"net"
	"testing"
	"time"
)

type fakeNetResolver struct {
	ifaces   []net.Interface
	addrs    map[int][]net.Addr
	dialIP   string
	dialErr  error
	ifaceErr error
}

func (f fakeNetResolver) Interfaces() ([]net.Interface, error) {
	if f.ifaceErr != nil {
		return nil, f.ifaceErr
	}
	return f.ifaces, nil
}

func (f fakeNetResolver) InterfaceAddrs(iface net.Interface) ([]net.Addr, error) {
	return f.addrs[iface.Index], nil
}

func (f fakeNetResolver) Dial(_, _ string) (net.Conn, error) {
	if f.dialErr != nil {
		return nil, f.dialErr
	}
	return &fakeConn{addr: &net.UDPAddr{IP: net.ParseIP(f.dialIP), Port: 1234}}, nil
}

type fakeConn struct{ addr net.Addr }

func (c *fakeConn) Read(_ []byte) (int, error)         { return 0, nil }
func (c *fakeConn) Write(b []byte) (int, error)        { return len(b), nil }
func (c *fakeConn) Close() error                       { return nil }
func (c *fakeConn) LocalAddr() net.Addr                { return c.addr }
func (c *fakeConn) RemoteAddr() net.Addr               { return c.addr }
func (c *fakeConn) SetDeadline(_ time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(_ time.Time) error { return nil }

func TestResolveLocalIPv4_PrefersInterfaceAddress(t *testing.T) {
	resolver := fakeNetResolver{
		ifaces: []net.Interface{{Index: 1, Flags: net.FlagUp}},
		addrs: map[int][]net.Addr{
			1: {&net.IPNet{IP: net.ParseIP("10.1.2.3"), Mask: net.CIDRMask(24, 32)}},
		},
		dialIP: "192.168.1.10",
	}

	ip, err := resolveLocalIPv4(resolver, "8.8.8.8:80")
	if err != nil {
		t.Fatalf("resolveLocalIPv4() error: %v", err)
	}
	if ip != "10.1.2.3" {
		t.Fatalf("resolveLocalIPv4() = %q, want %q", ip, "10.1.2.3")
	}
}

func TestResolveLocalIPv4_FallsBackToProbe(t *testing.T) {
	resolver := fakeNetResolver{
		ifaces: []net.Interface{{Index: 1, Flags: net.FlagLoopback}},
		addrs:  map[int][]net.Addr{1: {}},
		dialIP: "172.16.0.9",
	}

	ip, err := resolveLocalIPv4(resolver, "8.8.8.8:80")
	if err != nil {
		t.Fatalf("resolveLocalIPv4() error: %v", err)
	}
	if ip != "172.16.0.9" {
		t.Fatalf("resolveLocalIPv4() = %q, want %q", ip, "172.16.0.9")
	}
}

// (a) Empty probe target + interface success: uses interface, does not probe.
func TestResolveLocalIPv4_EmptyProbeTarget_InterfaceSuccess(t *testing.T) {
	resolver := fakeNetResolver{
		ifaces: []net.Interface{{Index: 1, Flags: net.FlagUp}},
		addrs: map[int][]net.Addr{
			1: {&net.IPNet{IP: net.ParseIP("192.168.0.5"), Mask: net.CIDRMask(24, 32)}},
		},
		dialErr: errors.New("should not be called"),
	}

	ip, err := resolveLocalIPv4(resolver, "")
	if err != nil {
		t.Fatalf("resolveLocalIPv4() error: %v", err)
	}
	if ip != "192.168.0.5" {
		t.Fatalf("resolveLocalIPv4() = %q, want %q", ip, "192.168.0.5")
	}
}

// (b) Empty probe target + interface failure: returns error, does not fall back to probe.
func TestResolveLocalIPv4_EmptyProbeTarget_InterfaceFailure_ReturnsError(t *testing.T) {
	resolver := fakeNetResolver{
		ifaces:  []net.Interface{{Index: 1, Flags: net.FlagLoopback}},
		addrs:   map[int][]net.Addr{1: {}},
		dialErr: errors.New("should not be called"),
	}

	_, err := resolveLocalIPv4(resolver, "")
	if err == nil {
		t.Fatal("expected error when interface enumeration fails and probe target is empty")
	}
}

// (c) Explicit probe target + interface failure: falls back to probe successfully.
func TestResolveLocalIPv4_ExplicitProbeTarget_InterfaceFailure_ProbeSuccess(t *testing.T) {
	resolver := fakeNetResolver{
		ifaces: []net.Interface{{Index: 1, Flags: net.FlagLoopback}},
		addrs:  map[int][]net.Addr{1: {}},
		dialIP: "10.20.30.40",
	}

	ip, err := resolveLocalIPv4(resolver, "8.8.8.8:80")
	if err != nil {
		t.Fatalf("resolveLocalIPv4() error: %v", err)
	}
	if ip != "10.20.30.40" {
		t.Fatalf("resolveLocalIPv4() = %q, want %q", ip, "10.20.30.40")
	}
}
