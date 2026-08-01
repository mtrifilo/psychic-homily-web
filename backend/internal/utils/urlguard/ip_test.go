package urlguard

import (
	"net"
	"testing"
)

// TestIsPublicIP is the SSRF-guard core: only globally-routable addresses pass.
// Moved here from services/pipeline (PSY-1191) when PSY-1675 made this the one
// classifier both the liveness dialer and the image_url write guard consult.
func TestIsPublicIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		// Blocked — the SSRF attack surface.
		{"127.0.0.1", false},         // loopback
		{"::1", false},               // ipv6 loopback
		{"169.254.169.254", false},   // cloud metadata (link-local)
		{"10.0.0.5", false},          // RFC1918 private
		{"192.168.1.1", false},       // RFC1918 private
		{"172.16.0.1", false},        // RFC1918 private
		{"0.0.0.0", false},           // unspecified
		{"::", false},                // ipv6 unspecified
		{"fc00::1", false},           // ipv6 unique-local (private)
		{"fe80::1", false},           // ipv6 link-local
		{"224.0.0.1", false},         // multicast
		{"255.255.255.255", false},   // broadcast (240/4)
		{"::ffff:127.0.0.1", false},  // ipv4-mapped loopback (normalization)
		{"::ffff:10.0.0.1", false},   // ipv4-mapped private
		{"100.64.0.1", false},        // CGNAT (RFC 6598) — stdlib IsPrivate misses this
		{"100.127.255.255", false},   // CGNAT upper edge
		{"::ffff:100.64.0.1", false}, // ipv4-mapped CGNAT
		{"64:ff9b::1.1.1.1", false},  // NAT64 well-known prefix (embeds an IPv4 host)
		{"2002:7f00:1::", false},     // 6to4 embedding 127.0.0.1
		{"2001::1", false},           // Teredo
		{"::127.0.0.1", false},       // IPv4-compatible (::/96) embedding loopback
		{"::169.254.169.254", false}, // IPv4-compatible embedding cloud metadata
		{"::10.0.0.1", false},        // IPv4-compatible embedding RFC1918
		{"192.0.0.192", false},       // IETF protocol assignments (Oracle Cloud metadata)
		{"192.0.2.10", false},        // TEST-NET-1 documentation range
		{"198.51.100.10", false},     // TEST-NET-2
		{"203.0.113.10", false},      // TEST-NET-3
		{"198.18.0.10", false},       // benchmarking range
		{"192.88.99.1", false},       // 6to4 relay anycast
		// IPv6 shapes the old enumerate-the-bad-prefixes approach called public
		// and the positive 2000::/3 rule refuses without naming them.
		{"::ffff:0:7f00:1", false},   // IPv4-TRANSLATED (SIIT) embedding 127.0.0.1
		{"64:ff9b:1::7f00:1", false}, // NAT64 local-use prefix (RFC 8215)
		{"fec0::1", false},           // deprecated site-local (RFC 3879)
		{"100::1", false},            // discard-only (RFC 6666)
		{"5f00::1", false},           // RFC 9602
		// ...and the ones that DO sit inside 2000::/3, so the positive rule
		// alone would admit them and the CIDR list must subtract them.
		{"2001:db8::1", false}, // documentation (RFC 3849)
		{"2001:10::1", false},  // ORCHID
		{"2001:20::1", false},  // ORCHIDv2
		{"3fff::1", false},     // documentation (RFC 9637)
		// Allowed — real public hosts.
		{"1.1.1.1", true},
		{"8.8.8.8", true},
		{"93.184.216.34", true}, // example.com
		{"2606:2800:220:1:248:1893:25c8:1946", true},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("bad test IP %q", c.ip)
		}
		if got := IsPublicIP(ip); got != c.want {
			t.Errorf("IsPublicIP(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
}

// TestIsPublicIP_FailsClosedOnUnclassifiable covers the inputs a resolver should
// never produce but a caller must not treat as public if it does.
func TestIsPublicIP_FailsClosedOnUnclassifiable(t *testing.T) {
	if IsPublicIP(nil) {
		t.Error("nil IP must not be public")
	}
	if IsPublicIP(net.IP{1, 2, 3}) {
		t.Error("malformed 3-byte IP must not be public")
	}
}

// TestParseHostIP_NumericSpellings is the reason this helper exists: glibc's
// inet_aton (and therefore the fetchers downstream of a stored image_url)
// accepts spellings net.ParseIP rejects, and every one of them is the same
// loopback/metadata address.
func TestParseHostIP_NumericSpellings(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{"127.0.0.1", "127.0.0.1"},           // canonical dotted quad
		{"2130706433", "127.0.0.1"},          // decimal
		{"0x7f000001", "127.0.0.1"},          // hex
		{"0X7F000001", "127.0.0.1"},          // hex, upper case
		{"017700000001", "127.0.0.1"},        // octal, single part
		{"0177.0.0.1", "127.0.0.1"},          // octal leading part
		{"127.1", "127.0.0.1"},               // two-part short form
		{"127.0.1", "127.0.0.1"},             // three-part short form
		{"2852039166", "169.254.169.254"},    // decimal cloud metadata
		{"0xa9fea9fe", "169.254.169.254"},    // hex cloud metadata
		{"169.254.43518", "169.254.169.254"}, // three-part cloud metadata
		{"0", "0.0.0.0"},
		{"1.1.1.1", "1.1.1.1"}, // a public literal still parses; the range check decides
	}
	for _, c := range cases {
		got := ParseHostIP(c.host)
		if got == nil {
			t.Errorf("ParseHostIP(%q) = nil, want %s", c.host, c.want)
			continue
		}
		if !got.Equal(net.ParseIP(c.want)) {
			t.Errorf("ParseHostIP(%q) = %s, want %s", c.host, got, c.want)
		}
	}
}

// TestParseHostIP_IPv6 covers the bracketed and zone-bearing forms url.Hostname
// hands over.
func TestParseHostIP_IPv6(t *testing.T) {
	cases := []struct{ host, want string }{
		{"::1", "::1"},
		{"::ffff:169.254.169.254", "::ffff:169.254.169.254"},
		{"::ffff:a9fe:a9fe", "::ffff:169.254.169.254"},
		{"fe80::1%eth0", "fe80::1"},
		{"[::1]", "::1"},
	}
	for _, c := range cases {
		got := ParseHostIP(c.host)
		if got == nil || !got.Equal(net.ParseIP(c.want)) {
			t.Errorf("ParseHostIP(%q) = %v, want %s", c.host, got, c.want)
		}
	}
}

// TestParseHostIP_Names must return nil for anything that needs DNS, or the
// guard would classify real hosts as literals and never resolve them.
func TestParseHostIP_Names(t *testing.T) {
	for _, host := range []string{
		"example.com",
		"images.example.com",
		"0x0.example.com",
		"1.2.3.4.5",     // five parts is not an inet_aton address
		"1.2.3.256",     // out of range for its position
		"256.1.1.1",     // leading part over a byte
		"4294967296",    // one part over 32 bits
		"0x100000000",   // hex over 32 bits
		"09.0.0.1",      // 9 is not an octal digit
		"1..2",          // empty part
		"",              // no host
		"0x",            // bare prefix, no digits
		"0x7f_00_00_01", // Go's base-0 underscore grouping must not be honoured
		"127.0.0.1a",
	} {
		if got := ParseHostIP(host); got != nil {
			t.Errorf("ParseHostIP(%q) = %s, want nil (name)", host, got)
		}
	}
}
