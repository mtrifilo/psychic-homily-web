package pipeline

import (
	"net"
	"testing"
)

// TestIsPublicIP is the SSRF-guard core: only globally-routable addresses pass.
func TestIsPublicIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		// Blocked — the SSRF attack surface.
		{"127.0.0.1", false},        // loopback
		{"::1", false},              // ipv6 loopback
		{"169.254.169.254", false},  // cloud metadata (link-local)
		{"10.0.0.5", false},         // RFC1918 private
		{"192.168.1.1", false},      // RFC1918 private
		{"172.16.0.1", false},       // RFC1918 private
		{"0.0.0.0", false},          // unspecified
		{"::", false},               // ipv6 unspecified
		{"fc00::1", false},          // ipv6 unique-local (private)
		{"fe80::1", false},          // ipv6 link-local
		{"224.0.0.1", false},        // multicast
		{"::ffff:127.0.0.1", false}, // ipv4-mapped loopback (normalization)
		{"::ffff:10.0.0.1", false},  // ipv4-mapped private
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
		if got := isPublicIP(ip); got != c.want {
			t.Errorf("isPublicIP(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
}

// TestSSRFDialControl confirms the dial hook refuses non-public addresses and
// permits public ones.
func TestSSRFDialControl(t *testing.T) {
	if err := ssrfDialControl("tcp", "169.254.169.254:80", nil); err == nil {
		t.Errorf("ssrfDialControl must refuse the cloud-metadata address")
	}
	if err := ssrfDialControl("tcp", "127.0.0.1:443", nil); err == nil {
		t.Errorf("ssrfDialControl must refuse loopback")
	}
	if err := ssrfDialControl("tcp", "8.8.8.8:443", nil); err != nil {
		t.Errorf("ssrfDialControl must permit a public address, got %v", err)
	}
	if err := ssrfDialControl("tcp", "garbage", nil); err == nil {
		t.Errorf("ssrfDialControl must reject a malformed address")
	}
}

// TestIsLive_RejectsNonHTTPSchemes confirms the public IsLive entrypoint rejects
// non-network schemes before any transport work.
func TestIsLive_RejectsNonHTTPSchemes(t *testing.T) {
	c := NewSSRFSafeLivenessChecker()
	for _, u := range []string{"javascript:alert(1)", "file:///etc/passwd", "ftp://host/x", "", "://nohost"} {
		if c.IsLive(u) {
			t.Errorf("IsLive(%q) = true, want false", u)
		}
	}
}
