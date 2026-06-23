package pipeline

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// LivenessChecker reports whether a candidate URL is reachable. It is an
// interface so the discovery flow can be unit-tested without real network I/O
// (PSY-1191): the production implementation is SSRFSafeLivenessChecker.
type LivenessChecker interface {
	// IsLive returns true if rawURL responds with a non-server-error status.
	IsLive(rawURL string) bool
}

const (
	// livenessTimeout bounds a single liveness probe. The discover-music
	// endpoint fans out one probe per candidate link; keep it short so a slow
	// host can't stall the admin request.
	livenessTimeout = 6 * time.Second
	// livenessMaxRedirects caps redirect-following. Each hop is re-validated by
	// the SSRF dial guard, but a bound also prevents redirect loops from
	// consuming the whole timeout budget.
	livenessMaxRedirects = 3
)

// SSRFSafeLivenessChecker performs HEAD (falling back to GET) liveness probes
// through an HTTP client whose dialer rejects any connection to a non-public IP.
//
// SSRF defense (PSY-1191): the candidate URLs originate from MusicBrainz, not a
// trusted allowlist, and are fetched server-side — a classic SSRF surface. The
// guard lives at DIAL time (net.Dialer.Control), so it inspects the ACTUAL
// resolved IP for every connection the client makes, including IPs reached via
// HTTP redirects and DNS-rebinding. A hostile or compromised value that resolves
// to 127.0.0.1, 169.254.169.254 (cloud metadata), a private RFC1918 range, or a
// link-local address is refused before a single byte is sent. Host-substring
// allowlisting alone (the codebase's stored-URL convention) cannot defend a
// fetch path, because DNS resolution happens after the host check.
type SSRFSafeLivenessChecker struct {
	client    *http.Client
	userAgent string
}

// NewSSRFSafeLivenessChecker builds the production liveness checker with the
// SSRF-guarded dialer wired in.
func NewSSRFSafeLivenessChecker() *SSRFSafeLivenessChecker {
	dialer := &net.Dialer{
		Timeout: livenessTimeout,
		// Control runs after DNS resolution, before connect, for every dialed
		// address. Returning an error here aborts the connection.
		Control: ssrfDialControl,
	}
	// DialContext is the guarded dialer. http.Transport builds TLS connections
	// on top of this same DialContext (it has no separate DialTLSContext set),
	// so the IP guard covers https targets too.
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   livenessTimeout,
		ResponseHeaderTimeout: livenessTimeout,
		DisableKeepAlives:     true,
	}
	client := &http.Client{
		Timeout:   livenessTimeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= livenessMaxRedirects {
				return fmt.Errorf("stopped after %d redirects", livenessMaxRedirects)
			}
			return nil
		},
	}
	return &SSRFSafeLivenessChecker{
		client:    client,
		userAgent: mbUserAgent,
	}
}

// IsLive reports whether rawURL is reachable. It first re-validates the scheme
// (http/https) so a non-network scheme can't reach the transport, then issues a
// HEAD; if the host rejects HEAD (405/501) it retries with a ranged GET. A
// 2xx/3xx/4xx response counts as "live" (the resource exists / the host
// answered); a transport error or 5xx counts as not live. A dial-guard refusal
// surfaces as a transport error → not live, which is the safe default.
func (c *SSRFSafeLivenessChecker) IsLive(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}

	if status, ok := c.probe(http.MethodHead, rawURL); ok {
		if status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented {
			// Host doesn't allow HEAD — fall through to GET.
			if getStatus, getOK := c.probe(http.MethodGet, rawURL); getOK {
				return statusIsLive(getStatus)
			}
			return false
		}
		return statusIsLive(status)
	}
	return false
}

// probe issues a single request and returns the response status. ok is false on
// any transport-level failure (including an SSRF dial refusal).
func (c *SSRFSafeLivenessChecker) probe(method, rawURL string) (status int, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), livenessTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("User-Agent", c.userAgent)
	if method == http.MethodGet {
		// Only need to confirm the host answers — avoid pulling a full body.
		req.Header.Set("Range", "bytes=0-0")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure
	return resp.StatusCode, true
}

// statusIsLive treats any non-5xx status as "the host answered for this URL".
// 2xx/3xx are clearly live; 4xx (other than the HEAD-not-allowed codes handled
// upstream) still means the host is reachable and answered — a 404 link is
// surfaced to the admin as not-live via the candidate's own host-anchor gate
// elsewhere, but a transient 403/429 shouldn't drop an otherwise-valid match.
func statusIsLive(status int) bool {
	return status < 500
}

// ssrfDialControl is the net.Dialer.Control hook. address is "host:port" where
// host is already a resolved IP literal (the resolver runs before Control). It
// refuses any connection whose target IP is not a routable public address.
func ssrfDialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("ssrf guard: malformed dial address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// Control always receives a resolved IP literal; a non-IP here is
		// anomalous — fail closed.
		return fmt.Errorf("ssrf guard: non-IP dial host %q", host)
	}
	if !isPublicIP(ip) {
		return fmt.Errorf("ssrf guard: refusing to dial non-public address %s", ip)
	}
	return nil
}

// isPublicIP reports whether ip is a globally-routable public address — i.e. NOT
// loopback, private (RFC1918 / RFC4193), link-local (incl. 169.254.169.254
// cloud metadata), multicast, unspecified (0.0.0.0 / ::), or an
// IPv4-in-IPv6-mapped form of any of those. This is the allowlist-by-exclusion
// at the heart of the SSRF guard.
func isPublicIP(ip net.IP) bool {
	// Normalize an IPv4-mapped IPv6 address (::ffff:127.0.0.1) to its IPv4 form
	// so the IPv4 checks below apply.
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
	// Block the IPv6 unique-local range fc00::/7 (covered by IsPrivate in Go
	// 1.17+, but assert explicitly for clarity) and the documentation/test
	// ranges are not relevant here.
	return true
}
