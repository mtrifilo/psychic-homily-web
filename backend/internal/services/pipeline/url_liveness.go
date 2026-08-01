package pipeline

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"psychic-homily-backend/internal/utils/urlguard"
)

// LivenessChecker reports whether a candidate URL is reachable. It is an
// interface so the discovery flow can be unit-tested without real network I/O
// (PSY-1191): the production implementation is SSRFSafeLivenessChecker. The
// context lets the caller's request deadline cancel an in-flight probe.
type LivenessChecker interface {
	// IsLive returns true if rawURL responds with a non-server-error status.
	IsLive(ctx context.Context, rawURL string) bool
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
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= livenessMaxRedirects {
				return fmt.Errorf("stopped after %d redirects", livenessMaxRedirects)
			}
			// Re-anchor every redirect HOP to an allowed platform host. The
			// dial-time IP guard already refuses non-public targets, but
			// host-anchoring the redirect chain is defense-in-depth: it keeps the
			// probe from being laundered through an open redirect on bandcamp/
			// spotify to an arbitrary public third party (request forgery from the
			// server's IP), and it means the IP guard isn't the SOLE control.
			if !isAllowedPlatformHost(req.URL.Hostname()) {
				return fmt.Errorf("refusing redirect to non-platform host %q", req.URL.Hostname())
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
func (c *SSRFSafeLivenessChecker) IsLive(ctx context.Context, rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}

	if status, ok := c.probe(ctx, http.MethodHead, rawURL); ok {
		if status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented {
			// Host doesn't allow HEAD — fall through to GET.
			if getStatus, getOK := c.probe(ctx, http.MethodGet, rawURL); getOK {
				return statusIsLive(getStatus)
			}
			return false
		}
		return statusIsLive(status)
	}
	return false
}

// isAllowedPlatformHost reports whether host is bandcamp.com (or a bandcamp
// artist subdomain) or open.spotify.com. It is the single host allowlist for the
// discover-music flow: classifyPlatformURL gates candidate URLs on it, and the
// liveness client's CheckRedirect re-anchors every redirect hop on it. Host is
// compared case-insensitively; callers pass url.Hostname() (no port, no
// brackets).
func isAllowedPlatformHost(host string) bool {
	h := strings.ToLower(host)
	return h == "bandcamp.com" || strings.HasSuffix(h, ".bandcamp.com") || h == "open.spotify.com"
}

// probe issues a single request and returns the response status. ok is false on
// any transport-level failure (including an SSRF dial refusal). The probe is
// bounded by the smaller of livenessTimeout and the caller's ctx deadline, so a
// disconnected admin request cancels in-flight probes instead of running to
// completion.
func (c *SSRFSafeLivenessChecker) probe(ctx context.Context, method, rawURL string) (status int, ok bool) {
	ctx, cancel := context.WithTimeout(ctx, livenessTimeout)
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
	// Drain a bounded prefix before close so the connection can be reused/torn
	// down cleanly even on the GET fallback (a non-Range-honoring host returns a
	// full body); the dial guard + DisableKeepAlives already cap exposure.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure
	return resp.StatusCode, true
}

// statusIsLive treats any non-5xx status as "the host answered for this URL".
// 2xx/3xx are clearly live; a 4xx still means the host is reachable and
// answered, so a transient 403/429 doesn't drop an otherwise-valid match. Only a
// transport error or a 5xx counts as not-live. (A genuine 404 artist page is
// rare for a host-anchored bandcamp/spotify link and the admin reviews the
// candidate regardless, so erring toward "answered" is the safer default.)
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
	if !urlguard.IsPublicIP(ip) {
		return fmt.Errorf("ssrf guard: refusing to dial non-public address %s", ip)
	}
	return nil
}

