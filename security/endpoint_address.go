package security

import "net/netip"

type addressClass int

const (
	addressPublic addressClass = iota
	// addressNonPublic needs an explicit EndpointRule.PrivateRanges grant.
	addressNonPublic
	// addressRefused is never a valid destination.
	addressRefused
)

// nonPublicBlocks are internal destinations an operator may grant: private, loopback, link-local, shared address space (carrier-grade NAT and some cloud metadata services), IETF protocol assignments, benchmarking and local-use NAT64.
var nonPublicBlocks = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
}

// refusedBlocks are never routable endpoints: "this network", documentation, reserved, IPv4-compatible IPv6, discard-only, documentation IPv6 and Teredo.
var refusedBlocks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2001::/32"),
}

var (
	nat64Block  = netip.MustParsePrefix("64:ff9b::/96")
	sixToFour   = netip.MustParsePrefix("2002::/16")
	ipv4Mapping = netip.MustParsePrefix("::ffff:0:0/96")
)

// classifyAddress decides whether address is public, needs a private grant, or is refused. IPv6 forms that embed an IPv4 destination (NAT64 and 6to4) take the class of the embedded address, and are refused when that address is not public, since a private grant cannot name it.
func classifyAddress(address netip.Addr) addressClass {
	if ipv4Mapping.Contains(address) {
		address = address.Unmap()
	}
	if !address.IsValid() || address.Zone() != "" || address.IsUnspecified() || address.IsMulticast() {
		return addressRefused
	}
	if embedded, ok := embeddedIPv4(address); ok {
		if classifyAddress(embedded) != addressPublic {
			return addressRefused
		}
		return addressPublic
	}
	if inAny(nonPublicBlocks, address) {
		return addressNonPublic
	}
	if inAny(refusedBlocks, address) || !address.IsGlobalUnicast() {
		return addressRefused
	}
	return addressPublic
}

func embeddedIPv4(address netip.Addr) (netip.Addr, bool) {
	b := address.As16()
	switch {
	case !address.Is6():
		return netip.Addr{}, false
	case nat64Block.Contains(address):
		return netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}), true
	case sixToFour.Contains(address):
		return netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}), true
	default:
		return netip.Addr{}, false
	}
}

func inAny(blocks []netip.Prefix, address netip.Addr) bool {
	for _, block := range blocks {
		if block.Contains(address) {
			return true
		}
	}
	return false
}
