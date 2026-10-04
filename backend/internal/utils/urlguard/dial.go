package urlguard

import (
	"fmt"
	"net"
	"syscall"
)

// DialControl is a net.Dialer.Control hook for any server-side fetch of a URL
// that did not come from a trusted allowlist. It runs after DNS resolution and
// before connect, for every dialed address including redirect hops, so address
// is "host:port" with host already a resolved IP literal. It refuses any
// connection whose target is not a routable public address (IsPublicIP), which
// covers loopback, private ranges, link-local, and cloud metadata addresses
// reached directly, through a redirect, or through DNS rebinding.
func DialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("ssrf guard: malformed dial address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// Control always receives a resolved IP literal; a non-IP here is
		// anomalous, so fail closed.
		return fmt.Errorf("ssrf guard: non-IP dial host %q", host)
	}
	if !IsPublicIP(ip) {
		return fmt.Errorf("ssrf guard: refusing to dial non-public address %s", ip)
	}
	return nil
}
