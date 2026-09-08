package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/httprate"
)

// TestCanonicalizeRateLimitKey_IPv4IsVerbatim: an IPv4 key is the address
// itself. Every IPv4 fixture in this package depends on it, and a normalisation
// applied here would silently re-bucket every existing IPv4 client.
func TestCanonicalizeRateLimitKey_IPv4IsVerbatim(t *testing.T) {
	for _, ip := range []string{"203.0.113.9", "198.51.100.1", "10.0.0.1", "127.0.0.1", "0.0.0.0"} {
		if got := canonicalizeRateLimitKey(ip); got != ip {
			t.Errorf("canonicalizeRateLimitKey(%q) = %q, want the address unchanged", ip, got)
		}
	}
}

// TestCanonicalizeRateLimitKey_IPv6CollapsesToThePrefix: an IPv6 client holds a
// whole /64 and can pick any address in it per request, so the key has to be the
// prefix. Different /64s must stay in different buckets, or unrelated
// subscribers meter as one.
func TestCanonicalizeRateLimitKey_IPv6CollapsesToThePrefix(t *testing.T) {
	cases := []struct {
		name string
		ip   string
		want string
	}{
		{name: "low bits masked off", ip: "2001:db8:1:2:3:4:5:6", want: "2001:db8:1:2::"},
		{name: "another address in the same /64", ip: "2001:db8:1:2:ffff:ffff:ffff:ffff", want: "2001:db8:1:2::"},
		{name: "the prefix itself is already canonical", ip: "2001:db8:1:2::", want: "2001:db8:1:2::"},
		{name: "a neighbouring /64 is a different bucket", ip: "2001:db8:1:3::1", want: "2001:db8:1:3::"},
		{name: "uppercase and expanded forms canonicalise alike", ip: "2001:0DB8:0001:0002:0000:0000:0000:0009", want: "2001:db8:1:2::"},
		{name: "loopback", ip: "::1", want: "::"},
		{name: "unspecified", ip: "::", want: "::"},
		{name: "link-local", ip: "fe80::1ff:fe23:4567:890a", want: "fe80::"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canonicalizeRateLimitKey(tc.ip); got != tc.want {
				t.Errorf("canonicalizeRateLimitKey(%q) = %q, want %q", tc.ip, got, tc.want)
			}
		})
	}
}

// TestCanonicalizeRateLimitKey_IPv4MappedKeysOnTheQuad guards the trap in
// masking an IPv4-mapped address as IPv6: ::ffff:a.b.c.d has 96 leading bits
// fixed, so a /64 mask sends EVERY such address to one key. Some proxies emit
// this form on dual-stack sockets, which would put unrelated IPv4 clients in a
// single bucket and let any one of them exhaust it for the rest.
func TestCanonicalizeRateLimitKey_IPv4MappedKeysOnTheQuad(t *testing.T) {
	first := canonicalizeRateLimitKey("::ffff:203.0.113.9")
	if first != "203.0.113.9" {
		t.Errorf("mapped address keyed as %q, want the dotted quad 203.0.113.9", first)
	}

	second := canonicalizeRateLimitKey("::ffff:198.51.100.4")
	if second == first {
		t.Fatalf("two mapped IPv4 clients share the key %q; every dual-stack client would meter as one", first)
	}

	if plain := canonicalizeRateLimitKey("203.0.113.9"); plain != first {
		t.Errorf("::ffff:203.0.113.9 keyed as %q but 203.0.113.9 keyed as %q; one client, two buckets", first, plain)
	}
}

// TestCanonicalizeRateLimitKey_UnparseableIsPassedThrough: a value that is not
// an address still has to meter as itself. Collapsing it to a constant would put
// every such caller in one bucket.
func TestCanonicalizeRateLimitKey_UnparseableIsPassedThrough(t *testing.T) {
	for _, in := range []string{"", "not-an-ip", "pipe", "2001:db8::1/64"} {
		if got := canonicalizeRateLimitKey(in); got != in {
			t.Errorf("canonicalizeRateLimitKey(%q) = %q, want it unchanged", in, got)
		}
	}
}

// TestKeyByClientIP_IPv6RotationInsideAPrefixSharesABucket states the fix on the
// header path an attacker actually reaches.
func TestKeyByClientIP_IPv6RotationInsideAPrefixSharesABucket(t *testing.T) {
	t.Setenv(TrustedProxyHopsEnvVar, "1")

	key := func(clientIP string) string {
		t.Helper()
		r := httptest.NewRequest("POST", "/auth/login", nil)
		r.RemoteAddr = "10.0.0.1:4000"
		r.Header.Set("X-Forwarded-For", clientIP)
		got, err := KeyByClientIP(r)
		if err != nil {
			t.Fatalf("KeyByClientIP(%s): %v", clientIP, err)
		}
		return got
	}

	a := key("2001:db8:abcd:1::1")
	b := key("2001:db8:abcd:1:dead:beef:cafe:f00d")
	if a != b {
		t.Errorf("two addresses in one /64 keyed %q and %q; rotating the low 64 bits still mints fresh buckets", a, b)
	}

	if other := key("2001:db8:abcd:2::1"); other == a {
		t.Errorf("a different /64 shares the key %q; unrelated subscribers would meter as one", a)
	}
}

// TestKeyByClientIP_BothPathsAgree: the header path and the RemoteAddr fallback
// must produce one key for one client, or a topology change silently hands every
// client a second budget.
func TestKeyByClientIP_BothPathsAgree(t *testing.T) {
	t.Setenv(TrustedProxyHopsEnvVar, "1")

	viaHeader := func(clientIP string) string {
		t.Helper()
		r := httptest.NewRequest("POST", "/auth/login", nil)
		r.RemoteAddr = "10.0.0.1:4000"
		r.Header.Set("X-Forwarded-For", clientIP)
		got, _ := KeyByClientIP(r)
		return got
	}
	viaRemoteAddr := func(hostPort string) string {
		t.Helper()
		r := httptest.NewRequest("POST", "/auth/login", nil)
		r.RemoteAddr = hostPort
		got, _ := KeyByClientIP(r)
		return got
	}

	cases := []struct {
		clientIP string
		hostPort string
	}{
		{clientIP: "198.51.100.7", hostPort: "198.51.100.7:1234"},
		{clientIP: "2001:db8:abcd:1::1", hostPort: "[2001:db8:abcd:1::1]:1234"},
		{clientIP: "2001:db8:abcd:1:2:3:4:5", hostPort: "[2001:db8:abcd:1::1]:1234"},
		{clientIP: "::ffff:198.51.100.7", hostPort: "198.51.100.7:1234"},
	}
	for _, tc := range cases {
		if h, ra := viaHeader(tc.clientIP), viaRemoteAddr(tc.hostPort); h != ra {
			t.Errorf("client %s keyed %q through the header and %q through RemoteAddr", tc.clientIP, h, ra)
		}
	}
}

// TestKeyByClientIP_LimiterMetersThePrefix runs the key through a real httprate
// limiter, the only assertion here that covers the whole chain: an IPv6 client
// cannot buy extra budget by changing address inside its own prefix, and a client
// in another prefix still arrives with a full one.
func TestKeyByClientIP_LimiterMetersThePrefix(t *testing.T) {
	t.Setenv(TrustedProxyHopsEnvVar, "1")

	const budget = 3
	limited := httprate.Limit(budget, time.Minute, httprate.WithKeyFuncs(KeyByClientIP))(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
	)

	attempt := func(clientIP string) int {
		r := httptest.NewRequest("POST", "/auth/login", nil)
		r.RemoteAddr = "10.0.0.1:4000"
		r.Header.Set("X-Forwarded-For", clientIP)
		w := httptest.NewRecorder()
		limited.ServeHTTP(w, r)
		return w.Code
	}

	for i, ip := range []string{
		"2001:db8:5:5::1",
		"2001:db8:5:5::2",
		"2001:db8:5:5:aaaa::9",
	} {
		if code := attempt(ip); code != http.StatusOK {
			t.Fatalf("attempt %d from %s: want 200 got %d", i+1, ip, code)
		}
	}
	if code := attempt("2001:db8:5:5:ffff::1"); code != http.StatusTooManyRequests {
		t.Errorf("a fourth address in the same /64: want 429 got %d; the budget is per address, not per client", code)
	}

	if code := attempt("2001:db8:5:6::1"); code != http.StatusOK {
		t.Errorf("first request from a different /64: want 200 got %d; unrelated subscribers share a bucket", code)
	}
	if code := attempt("198.51.100.44"); code != http.StatusOK {
		t.Errorf("first request from an IPv4 client: want 200 got %d", code)
	}
}
