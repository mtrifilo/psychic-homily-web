package urlguard

import (
	"errors"
	"fmt"
	"net"
	"syscall"
)

// ErrNonPublicAddress is wrapped by every DialControl refusal of a resolved
// address, so a caller can tell "this host is not public" (permanent) from a
// network failure.
var ErrNonPublicAddress = errors.New("non-public address")

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
		return fmt.Errorf("ssrf guard: refusing to dial %w %s", ErrNonPublicAddress, ip)
	}
	return nil
}
