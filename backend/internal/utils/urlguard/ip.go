package urlguard

import (
	"net"
	"strconv"
	"strings"
)

// blockedNets enumerates the non-public ranges Go's stdlib IP predicates
// (IsLoopback/IsPrivate/IsLinkLocal*/IsMulticast/IsUnspecified) DON'T cover but
// an SSRF guard must still refuse. The stdlib predicates are applied separately
// in IsPublicIP; this list closes their gaps. Every entry is checked against
// BOTH the original IP and the To4()-normalized form, so an IPv4-mapped IPv6
// wrapper (::ffff:a.b.c.d) of any IPv4 range here is also caught.
var blockedNets = mustCIDRs(
	// IPv4 ranges stdlib misses:
	"0.0.0.0/8",       // "this host on this network" (RFC 1122) — 0.x dials localhost on Linux
	"100.64.0.0/10",   // CGNAT shared address space (RFC 6598)
	"192.0.0.0/24",    // IETF protocol assignments (incl. Oracle Cloud's 192.0.0.192 metadata host)
	"192.0.2.0/24",    // TEST-NET-1 (RFC 5737 documentation)
	"198.18.0.0/15",   // benchmarking (RFC 2544)
	"198.51.100.0/24", // TEST-NET-2
	"203.0.113.0/24",  // TEST-NET-3
	"192.88.99.0/24",  // 6to4 relay anycast (RFC 7526) — reaches an arbitrary 6to4 relay
	"240.0.0.0/4",     // reserved class E (incl. 255.255.255.255 broadcast)
	// IPv6 prefixes that sit INSIDE global unicast (2000::/3) and so survive the
	// positive rule in IsPublicIP. Everything outside 2000::/3 — IPv4-compatible
	// ::/96, IPv4-translated ::ffff:0:0/96, both NAT64 prefixes, fec0::/10
	// site-local, 100::/64 discard, 5f00::/16 — is refused by that rule without
	// needing an entry here.
	"2002::/16",     // 6to4 — bits 16..48 are an embedded IPv4 host (2002:7f00::/24 = 127.0.0.0/8)
	"2001::/32",     // Teredo — relays to arbitrary IPv4
	"2001:db8::/32", // documentation (RFC 3849) — the IPv6 analogue of TEST-NET
	"2001:10::/28",  // ORCHID (RFC 4843, deprecated)
	"2001:20::/28",  // ORCHIDv2 (RFC 7343)
	"3fff::/20",     // documentation (RFC 9637)
)

func mustCIDRs(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic("invalid SSRF CIDR " + c + ": " + err.Error())
		}
		out = append(out, n)
	}
	return out
}

// IsPublicIP reports whether ip is a globally-routable public address. It is the
// allowlist-by-exclusion at the heart of every SSRF guard in this codebase: an
// address is public ONLY if it survives every block below. The blocked set is
// the union of (a) the stdlib predicates — loopback, private (RFC1918 / RFC4193
// fc00::/7), link-local (incl. 169.254.169.254 cloud metadata), multicast,
// unspecified — and (b) the blockedNets CIDR list, which closes the ranges those
// predicates miss (0.0.0.0/8, CGNAT, class-E/broadcast, documentation/benchmark,
// NAT64, 6to4, Teredo). Each blockedNets entry is checked against both the
// original form and the IPv4-mapped-normalized form, so a mapped wrapper of any
// blocked IPv4 range is also refused.
//
// A nil or malformed IP is NOT public: callers fail closed on anything they
// could not classify.
func IsPublicIP(ip net.IP) bool {
	if ip == nil || (len(ip) != net.IPv4len && len(ip) != net.IPv6len) {
		return false
	}
	// One blockedNets pass covers both shapes: net.IPNet.Contains normalizes an
	// IPv4-mapped address (::ffff:a.b.c.d) to its 4-byte form before comparing,
	// so a mapped wrapper of any IPv4 entry here matches the IPv4 entry, while a
	// genuine IPv6 address matches only the IPv6 entries. (An earlier version
	// checked twice, before and after To4(), on the theory that the mapped form
	// slipped the first pass. It does not — the second pass was dead code.)
	if inAnyNet(ip, blockedNets) {
		return false
	}
	// Normalize an IPv4-mapped IPv6 address (::ffff:127.0.0.1) to its IPv4 form
	// so the stdlib IPv4 predicates apply — those do NOT normalize.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() {
		return false
	}
	// For IPv6, state the rule POSITIVELY: an address is public only if it is in
	// global unicast 2000::/3. Enumerating bad IPv6 prefixes loses — the list
	// above started as an enumeration and was still missing IPv4-translated
	// ::ffff:0:0/96, NAT64 local-use, fec0::/10 site-local, 100::/64 discard and
	// several documentation blocks, every one of which net.IP.IsGlobalUnicast
	// happily calls public (it only excludes loopback/multicast/link-local/
	// unspecified). One positive rule refuses all of them and every future
	// reserved block without naming any of them. This mirrors the edge guard's
	// isAllowedIPv6, and the blockedNets entries above then subtract the
	// embedding and documentation prefixes that live INSIDE 2000::/3, which the
	// edge's rule alone does not catch.
	if ip.To4() == nil {
		return len(ip) == net.IPv6len && ip[0]&0xe0 == 0x20
	}
	// For a normalized IPv4, To4() addresses are global-unicast unless caught
	// above.
	return ip.IsGlobalUnicast()
}

// inAnyNet reports whether ip falls within any of the given networks.
func inAnyNet(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ParseHostIP returns the address a URL host denotes when that host is an IP
// LITERAL in any form a resolver or a fetch implementation would accept, and nil
// when the host is a name that needs DNS.
//
// It exists because net.ParseIP alone is not a sufficient literal test at a
// write boundary. net.ParseIP accepts only the canonical dotted quad, but the
// stored value is later parsed by the WHATWG URL parser in the edge renderer,
// whose host parser implements inet_aton's grammar: it CANONICALISES the
// decimal (2130706433), octal (0177.0.0.1), hex (0x7f000001) and short-form
// (127.1) spellings to 127.0.0.1 before the fetch. Those spellings therefore
// reach loopback whatever this side calls them.
//
// What this buys is determinism and a truthful error, not the bare difference
// between stored and refused: without it those hosts fall through to the DNS
// path, where a cgo resolver answers 127.0.0.1 (rejected by address) and the
// pure-Go resolver fails to look them up. Classifying them here means the
// verdict does not depend on which resolver the binary was built with.
//
// The IPv6 branch drops a zone id (fe80::1%eth0) before parsing, since a zone
// never changes which range an address belongs to.
func ParseHostIP(host string) net.IP {
	host = strings.Trim(host, "[]")
	if host == "" {
		return nil
	}
	if strings.Contains(host, ":") {
		if i := strings.IndexByte(host, '%'); i >= 0 {
			host = host[:i]
		}
		return net.ParseIP(host)
	}
	return parseIPv4Numeric(host)
}

// parseIPv4Numeric implements inet_aton's grammar: one to four numeric parts,
// each in decimal, octal (leading 0) or hex (leading 0x), where the LAST part
// fills all the remaining low-order bytes. So 127.1 is 127.0.0.1 and
// 0x7f000001 is the same address again.
//
// Returns nil for anything that is not fully numeric, which is every real
// hostname: a DNS label containing a non-digit fails the per-part parse, so
// "example.com" and "0x0.example.com" both fall through to the name path.
func parseIPv4Numeric(host string) net.IP {
	parts := strings.Split(host, ".")
	if len(parts) > 4 {
		return nil
	}
	vals := make([]uint64, 0, 4)
	for _, p := range parts {
		v, ok := parseNumericPart(p)
		if !ok {
			return nil
		}
		vals = append(vals, v)
	}
	last := len(vals) - 1
	var addr uint64
	for i := 0; i < last; i++ {
		if vals[i] > 0xff {
			return nil
		}
		addr |= vals[i] << uint(8*(3-i))
	}
	// The final part occupies every byte the leading parts did not claim.
	remainingBits := uint(8 * (4 - last))
	if vals[last] >= uint64(1)<<remainingBits {
		return nil
	}
	addr |= vals[last]
	return net.IPv4(byte(addr>>24), byte(addr>>16), byte(addr>>8), byte(addr))
}

// parseNumericPart parses one inet_aton part. Base is chosen the way inet_aton
// chooses it: "0x"/"0X" prefix is hex, a leading zero is octal, otherwise
// decimal. An explicit base is passed to ParseUint so Go's base-0 underscore
// grouping ("0x7f_00_00_01") cannot sneak in a form no resolver accepts.
func parseNumericPart(s string) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	base := 10
	digits := s
	switch {
	case len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X'):
		base, digits = 16, s[2:]
	case len(s) > 1 && s[0] == '0':
		base, digits = 8, s[1:]
	}
	v, err := strconv.ParseUint(digits, base, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
