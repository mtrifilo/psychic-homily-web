package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/httprate"
)

// TrustedProxyHops is how many proxies sit in front of this service and append
// to X-Forwarded-For. Railway terminates TLS at its edge (the `railway-hikari`
// server header) and forwards to the container, so exactly one.
//
// This number is the whole security boundary of KeyByClientIP — see its doc.
const TrustedProxyHops = 1

// KeyByClientIP derives the rate-limit bucket key from the real client IP.
//
// # Why not httprate.KeyByIP (what every limiter used before PSY-1608)
//
// KeyByIP keys on r.RemoteAddr. Behind Railway's edge that is the PROXY's
// address, not the client's — so buckets are per-edge-node and shared by every
// client routed through it. Measured on production: three edge nodes each held
// their own counter, each decrementing independently, and a single client could
// never exhaust one.
//
// That is not merely a weak limit. It is shared fate: once traffic grows, one
// abuser exhausts the bucket for every legitimate user on the same edge node,
// and the auth limiter (5/min) would lock out the site. The limiter was
// simultaneously unable to stop an attacker and able to lock out everyone else.
//
// # Why not httprate.KeyByRealIP either
//
// KeyByRealIP honours True-Client-IP and X-Real-IP unconditionally, and takes
// the LEFTMOST X-Forwarded-For entry. All three are client-supplied. An
// attacker sets a different value per request and mints a fresh bucket every
// time — the same failure we are fixing, but deliberate and unbounded.
//
// # What this does instead
//
// X-Forwarded-For accumulates left to right: each proxy APPENDS the address it
// received the connection from. So with N trusted proxies in front, the entry
// at position len-N is the address OUR trusted proxy actually observed.
// Anything to the left of it was supplied by the caller and is worthless.
//
//	X-Forwarded-For: <spoofed>, <spoofed>, <real client>
//	                                        ^ what Railway observed, index len-1
//
// A client can prepend entries but cannot stop the trusted proxy appending the
// address it genuinely saw, so counting from the RIGHT is spoof-resistant while
// counting from the left is not.
//
// Falls back to RemoteAddr when the header is absent or malformed, which is the
// previous behaviour — so a misconfigured proxy degrades to the old bucket
// rather than to no limiting at all.
func KeyByClientIP(r *http.Request) (string, error) {
	if ip := clientIPFromForwardedFor(r.Header.Get("X-Forwarded-For"), TrustedProxyHops); ip != "" {
		return ip, nil
	}
	return httprate.KeyByIP(r)
}

// clientIPFromForwardedFor returns the address the trusted proxy observed, or
// "" when the header cannot be trusted to supply one.
//
// Exported behaviour is covered by tests rather than callers; kept separate so
// the parsing is testable without constructing requests.
func clientIPFromForwardedFor(header string, trustedHops int) string {
	if header == "" || trustedHops < 1 {
		return ""
	}

	parts := strings.Split(header, ",")
	idx := len(parts) - trustedHops
	if idx < 0 {
		// Fewer hops present than we trust: the chain is shorter than expected
		// (direct connection, or a proxy that does not append). Do NOT fall back
		// to a further-left entry — that is the spoofable region.
		return ""
	}

	ip := strings.TrimSpace(parts[idx])
	if ip == "" {
		return ""
	}
	// Some proxies append host:port rather than a bare address.
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	if net.ParseIP(ip) == nil {
		return ""
	}
	return ip
}
