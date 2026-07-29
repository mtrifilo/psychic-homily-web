package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/httprate"
)

// TrustedProxyHopsEnvVar tunes how many proxies are assumed to sit in front of
// this service and append to X-Forwarded-For.
//
// It is an ENV VAR rather than a constant on purpose. The hop count is a
// property of the DEPLOYMENT TOPOLOGY, not of the code, and getting it wrong
// fails silently in both directions — too few trusts a caller-supplied entry,
// too many falls back to the proxy address and stops limiting per client.
// Making it tunable means a topology change (a new CDN, an added load balancer)
// is a config change verified by observation, rather than a code change shipped
// on a guess. PSY-1608 shipped on a guess and did not move production at all.
const TrustedProxyHopsEnvVar = "RATE_LIMIT_TRUSTED_PROXY_HOPS"

// defaultTrustedProxyHops is 2, MEASURED rather than assumed.
//
// The `ratelimit_proxy_trust` observation below reports xff_chain_length=2 on
// BOTH Railway environments (production and stage), with no caller-supplied
// header — so two hops append before the request reaches the container, and the
// real client IP sits at index len-2.
//
// This was 1 first, on the reasoning that "Railway terminates TLS at its edge,
// so exactly one". That was wrong, and wrong silently: the limiter kept working
// on the intermediate proxy's address, buckets stayed shared, and a burst never
// hit 429. Verified after the correction — a single client now counts 4 3 2 1 0
// and is then rejected, on both the report and auth limiters.
//
// Environments WITHOUT a proxy (local dev, CI/E2E) are unaffected either way:
// X-Forwarded-For is absent, so the key falls back to RemoteAddr regardless of
// this value. An environment behind a DIFFERENT number of hops should set
// RATE_LIMIT_TRUSTED_PROXY_HOPS rather than change this default, and the
// observation log tells it which number to use.
const defaultTrustedProxyHops = 2

func trustedProxyHops() int {
	if v := os.Getenv(TrustedProxyHopsEnvVar); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			return n
		}
	}
	return defaultTrustedProxyHops
}

// KeyByClientIP derives the rate-limit bucket key from the real client IP.
//
// # Why not httprate.KeyByIP
//
// KeyByIP keys on r.RemoteAddr. Behind a proxy that is the PROXY's address, so
// buckets are per-proxy-node and shared by every client routed through one.
// Measured on production: counters bounced per Railway edge node and a single
// client could never exhaust one. That is not merely a weak limit — it is
// shared fate, where one abuser exhausts the budget for everyone routed through
// that node while not being stopped themselves.
//
// # Why not httprate.KeyByRealIP
//
// It honours True-Client-IP and X-Real-IP unconditionally and takes the
// LEFTMOST X-Forwarded-For entry. All three are caller-supplied: an attacker
// varies one per request and mints a fresh bucket every time.
//
// # What this does
//
// X-Forwarded-For accumulates left to right — each proxy APPENDS the address it
// received the connection from. With N trusted proxies, the entry at len-N is
// what OUR trusted proxy actually observed; everything to its left came from
// the caller.
//
//	X-Forwarded-For: <spoofed>, <spoofed>, <real client>
//	                                        ^ index len-N, N=1
//
// Falls back to RemoteAddr when the header is absent or unusable, so a
// misconfigured proxy degrades to the previous behaviour rather than to no
// limiting at all.
func KeyByClientIP(r *http.Request) (string, error) {
	xff := r.Header.Get("X-Forwarded-For")
	hops := trustedProxyHops()

	if ip := clientIPFromForwardedFor(xff, hops); ip != "" {
		observeProxyTrust(r, xff, hops, "x-forwarded-for", ip)
		return ip, nil
	}

	key, err := httprate.KeyByIP(r)
	observeProxyTrust(r, xff, hops, "remote-addr", key)
	return key, err
}

var proxyTrustOnce sync.Once

// observeProxyTrust reports, ONCE per process, what the limiter actually sees.
//
// This exists because PSY-1608 was diagnosed twice from the outside and the
// readings could not distinguish between "the header is absent", "the header
// varies per request", and "the trusted hop count is wrong". A fix shipped on
// the resulting inference changed nothing. One log line settles which of those
// is true — and settles it again for free the next time the topology changes.
//
// Logged once (sync.Once) rather than per request: this is a topology fact, not
// an event, so repeating it would be noise on a hot path.
//
// The derived key is HASHED and truncated. Comparing fingerprints across a burst
// answers the only question that matters — same bucket or different — without
// writing client IP addresses into logs.
func observeProxyTrust(r *http.Request, xff string, hops int, source, key string) {
	proxyTrustOnce.Do(func() {
		chain := 0
		if xff != "" {
			chain = len(strings.Split(xff, ","))
		}
		slog.Default().Info("ratelimit_proxy_trust",
			"source", source,
			"trusted_hops", hops,
			"xff_present", xff != "",
			"xff_chain_length", chain,
			"has_x_real_ip", r.Header.Get("X-Real-IP") != "",
			"has_true_client_ip", r.Header.Get("True-Client-IP") != "",
			"has_envoy_external", r.Header.Get("X-Envoy-External-Address") != "",
			"key_fingerprint", fingerprint(key),
		)
	})
}

// fingerprint reduces a bucket key to a short, non-reversible tag.
func fingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:4])
}

// clientIPFromForwardedFor returns the address the trusted proxy observed, or
// "" when the header cannot be trusted to supply one.
func clientIPFromForwardedFor(header string, trustedHops int) string {
	if header == "" || trustedHops < 1 {
		return ""
	}

	parts := strings.Split(header, ",")
	idx := len(parts) - trustedHops
	if idx < 0 {
		// Fewer hops present than we trust: the chain is shorter than expected
		// (direct connection, or a proxy that does not append). Do NOT fall back
		// to a further-left entry — that is the caller-controlled region.
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
