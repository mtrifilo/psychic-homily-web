package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
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
// misconfigured proxy degrades to the connection's own address rather than to no
// limiting at all. That fallback keys on the PROXY when a proxy is in front, so
// the two paths agree on one client only while the header path yields an address.
//
// # Canonical keys
//
// Both paths run the observed address through canonicalizeRateLimitKey before
// returning it, and the observation below reports that same key. The RemoteAddr
// path derives the host itself rather than calling httprate.KeyByIP, whose
// canonicalisation differs on IPv4-mapped addresses.
//
// The error is always nil. It exists because httprate.KeyFunc requires one, and
// a non-nil error fails the request with 428 instead of metering it.
func KeyByClientIP(r *http.Request) (string, error) {
	xff := r.Header.Get("X-Forwarded-For")
	hops := trustedProxyHops()

	if ip := clientIPFromForwardedFor(xff, hops); ip != "" {
		key := canonicalizeRateLimitKey(ip)
		observeProxyTrust(r, xff, hops, "x-forwarded-for", key)
		return key, nil
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	key := canonicalizeRateLimitKey(host)
	observeProxyTrust(r, xff, hops, "remote-addr", key)
	return key, nil
}

// rateLimitIPv6PrefixBits is the width of the IPv6 block that shares one
// rate-limit bucket.
//
// An IPv6 client is delegated at least a /64 and can source from any address
// inside it with no network change and no cost, so an address-exact key would be
// one bucket per REQUEST rather than one per client, and every per-IP limiter
// keyed by this function would be bypassed by rotating the low 64 bits. /64 is
// the smallest block a client is delegated, so masking further left would merge
// unrelated subscribers.
//
// Two consequences are accepted rather than solved here. Hosts that share one
// /64 because they share a LAN segment (an office, a campus, a VPN exit) share a
// bucket, which is the trade IPv4 NAT already makes. And a client delegated a
// block WIDER than a /64 still holds one bucket per /64 inside it, so this raises
// the cost of minting buckets without removing the ability.
const rateLimitIPv6PrefixBits = 64

// canonicalizeRateLimitKey maps a client address to the bucket it shares: an
// IPv4 address keys on itself, an IPv6 address on its rateLimitIPv6PrefixBits
// prefix.
//
// An IPv4-mapped IPv6 address (::ffff:203.0.113.9, which some proxies emit on
// dual-stack sockets) is unmapped first, so it keys on the dotted quad. Masking
// it as IPv6 instead would send EVERY such client to one key, because the form
// fixes the leading 96 bits. httprate.KeyByIP masks it; this deliberately does
// not.
//
// The other IPv4-in-IPv6 forms (::203.0.113.9, ::ffff:0:203.0.113.9) and the
// loopback and unspecified addresses all sit in ::/64 and so share the key "::".
// Nothing routable is addressed from ::/64, and httprate collapses them the same
// way.
//
// Input that does not parse as an address is returned unchanged rather than
// rejected: clientIPFromForwardedFor has already rejected anything net.ParseIP
// cannot read, and on the RemoteAddr path an unparseable value still has to meter
// as SOMETHING. Collapsing it to a constant would put every such client in one
// bucket.
func canonicalizeRateLimitKey(observed string) string {
	addr, err := netip.ParseAddr(observed)
	if err != nil {
		return observed
	}
	if addr = addr.Unmap(); addr.Is4() {
		return addr.String()
	}
	return netip.PrefixFrom(addr, rateLimitIPv6PrefixBits).Masked().Addr().String()
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
// The key is the canonical one the limiter meters on, hashed and truncated: the
// line identifies the bucket one sample landed in without writing a client IP
// address into logs.
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
