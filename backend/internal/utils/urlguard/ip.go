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
	"240.0.0.0/4",     // reserved class E (incl. 255.255.255.255 broadcast)
	// IPv6 ranges that embed/relay to arbitrary (incl. internal) IPv4 targets.
	// To4() normalizes only the IPv4-MAPPED form (::ffff:a.b.c.d), so these
	// embedding prefixes must be blocked explicitly on the original IPv6 shape:
	"::/96",        // IPv4-COMPATIBLE (deprecated) — ::a.b.c.d embeds an IPv4 host (::7f00:1 = 127.0.0.1). Does NOT match ::ffff:a.b.c.d (those normalize via To4).
	"64:ff9b::/96", // NAT64 well-known prefix — low 32 bits are an IPv4 host
	"2002::/16",    // 6to4 — bits 16..48 are an embedded IPv4 host (2002:7f00::/24 = 127.0.0.0/8)
	"2001::/32",    // Teredo — relays to arbitrary IPv4
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
	// Check blockedNets against the ORIGINAL form first, so an IPv6-shaped
	// embedding prefix (NAT64 / 6to4 / Teredo) is matched before To4() could
	// rewrite a mapped address into a bare IPv4.
	if inAnyNet(ip, blockedNets) {
		return false
	}
	// Normalize an IPv4-mapped IPv6 address (::ffff:127.0.0.1) to its IPv4 form
	// so the stdlib IPv4 predicates and the IPv4 entries in blockedNets apply.
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
	// Re-check blockedNets against the normalized IPv4 form (a ::ffff:100.64.x.x
	// mapped CGNAT address passes the pre-normalization check on its IPv6 shape).
	if inAnyNet(ip, blockedNets) {
		return false
	}
	// Final floor: only addresses the stdlib considers global-unicast are public.
	// This catches any remaining non-routable IPv6 shape (e.g. a future reserved
	// block) without an explicit CIDR. For a normalized IPv4, To4() addresses are
	// global-unicast unless caught above.
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
// value stored here is later handed to OTHER fetchers: glibc's getaddrinfo (and
// therefore Node/undici on the OG edge route) runs inet_aton, which also accepts
// the decimal (2130706433), octal (0177.0.0.1), hex (0x7f000001) and
// short-form (127.1) spellings of the SAME address. Classifying only the
// canonical spelling would store a value that our own guard called "a hostname"
// and the fetcher resolved to loopback.
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
