package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/httprate"
)

// TestCanonicalizeRateLimitKey_IPv4IsVerbatim: an IPv4 key is the address itself.
// A normalisation applied here would re-bucket every IPv4 client at deploy.
func TestCanonicalizeRateLimitKey_IPv4IsVerbatim(t *testing.T) {
	for _, ip := range []string{"203.0.113.9", "198.51.100.1", "10.0.0.1", "127.0.0.1", "0.0.0.0"} {
		if got := canonicalizeRateLimitKey(ip); got != ip {
			t.Errorf("canonicalizeRateLimitKey(%q) = %q, want the address unchanged", ip, got)
		}
	}
}

// TestKeyByClientIP_IPv4RemoteAddrFormsAreUnchanged pins the IPv4 keys of the
// RemoteAddr path against the forms httprate.KeyByIP used to handle there: with a
// port, without one, and a value that is not an address at all.
func TestKeyByClientIP_IPv4RemoteAddrFormsAreUnchanged(t *testing.T) {
	cases := []struct {
		remoteAddr string
		want       string
	}{
		{remoteAddr: "203.0.113.9:1234", want: "203.0.113.9"},
		{remoteAddr: "203.0.113.9", want: "203.0.113.9"},
		{remoteAddr: "not-an-address", want: "not-an-address"},
	}
	for _, tc := range cases {
		r := httptest.NewRequest("POST", "/auth/login", nil)
		r.RemoteAddr = tc.remoteAddr
		got, err := KeyByClientIP(r)
		if err != nil {
			t.Fatalf("KeyByClientIP(%q): %v", tc.remoteAddr, err)
		}
		if got != tc.want {
			t.Errorf("RemoteAddr %q keyed %q, want %q", tc.remoteAddr, got, tc.want)
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

// TestCanonicalizeRateLimitKey_NothingRoutableSharesTheZeroPrefix records the one
// prefix that holds several unrelated address forms: ::/64 carries loopback, the
// unspecified address, and the IPv4-compatible and IPv4-translated spellings, so
// all of them key on "::". None is a routable source address, and httprate
// collapses them the same way, so they share a bucket rather than getting a
// special case.
func TestCanonicalizeRateLimitKey_NothingRoutableSharesTheZeroPrefix(t *testing.T) {
	cases := []struct {
		name string
		ip   string
		want string
	}{
		{name: "loopback", ip: "::1", want: "::"},
		{name: "unspecified", ip: "::", want: "::"},
		{name: "IPv4-compatible", ip: "::203.0.113.9", want: "::"},
		{name: "IPv4-translated", ip: "::ffff:0:203.0.113.9", want: "::"},
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
	for _, in := range []string{"", "not-an-ip", "2001:db8::1/64", "203.0.113.9:1234"} {
		if got := canonicalizeRateLimitKey(in); got != in {
			t.Errorf("canonicalizeRateLimitKey(%q) = %q, want it unchanged", in, got)
		}
	}
}

// TestCanonicalizeRateLimitKey_ZoneIsDropped: a zone identifier names an
// interface on THIS host, not the client, so masking drops it and two zones of
// one link-local prefix key alike. clientIPFromForwardedFor rejects zoned entries
// before they reach here (net.ParseIP does not accept them), so this is reachable
// only from a RemoteAddr on a link-local listener.
func TestCanonicalizeRateLimitKey_ZoneIsDropped(t *testing.T) {
	for _, in := range []string{"fe80::1%eth0", "fe80::2%eth1"} {
		if got := canonicalizeRateLimitKey(in); got != "fe80::" {
			t.Errorf("canonicalizeRateLimitKey(%q) = %q, want fe80::", in, got)
		}
	}
}

// replacedKeys hold the two keying strategies KeyByClientIP no longer uses, so
// the tests below can state the difference as an assertion rather than a claim:
// the header path returning the observed address as-is, and httprate's
// canonicalisation of the RemoteAddr host.
var replacedKeys = struct {
	forwardedFor func(*http.Request) string
	remoteAddr   func(*http.Request) string
}{
	forwardedFor: func(r *http.Request) string {
		return clientIPFromForwardedFor(r.Header.Get("X-Forwarded-For"), trustedProxyHops())
	},
	remoteAddr: func(r *http.Request) string {
		key, _ := httprate.KeyByIP(r)
		return key
	},
}

// TestKeyByClientIP_IPv6RotationInsideAPrefixSharesABucket states the fix on the
// header path an attacker actually reaches, against the address-exact key it
// replaces.
func TestKeyByClientIP_IPv6RotationInsideAPrefixSharesABucket(t *testing.T) {
	t.Setenv(TrustedProxyHopsEnvVar, "1")

	req := func(clientIP string) *http.Request {
		r := httptest.NewRequest("POST", "/auth/login", nil)
		r.RemoteAddr = "10.0.0.1:4000"
		r.Header.Set("X-Forwarded-For", clientIP)
		return r
	}
	key := func(clientIP string) string {
		t.Helper()
		got, err := KeyByClientIP(req(clientIP))
		if err != nil {
			t.Fatalf("KeyByClientIP(%s): %v", clientIP, err)
		}
		return got
	}

	const (
		first  = "2001:db8:abcd:1::1"
		second = "2001:db8:abcd:1:dead:beef:cafe:f00d"
	)

	if a, b := replacedKeys.forwardedFor(req(first)), replacedKeys.forwardedFor(req(second)); a == b {
		t.Fatalf("the address-exact key collapsed %s and %s to %q; this test can no longer tell the two strategies apart", first, second, a)
	}

	a, b := key(first), key(second)
	if a != b {
		t.Errorf("two addresses in one /64 keyed %q and %q; rotating the low 64 bits still mints fresh buckets", a, b)
	}

	if other := key("2001:db8:abcd:2::1"); other == a {
		t.Errorf("a different /64 shares the key %q; unrelated subscribers would meter as one", a)
	}
}

// TestKeyByClientIP_IPv4MappedRemoteAddrIsNotCollapsed: httprate's
// canonicalisation masks ::ffff:a.b.c.d as IPv6, which fixes the leading 96 bits
// and sends every dual-stack client to the key "::". The RemoteAddr path must not
// inherit that, or one client exhausts the budget for all of them.
func TestKeyByClientIP_IPv4MappedRemoteAddrIsNotCollapsed(t *testing.T) {
	req := func(hostPort string) *http.Request {
		r := httptest.NewRequest("POST", "/auth/login", nil)
		r.RemoteAddr = hostPort
		return r
	}

	const (
		one = "[::ffff:203.0.113.9]:1234"
		two = "[::ffff:198.51.100.4]:1234"
	)

	if a, b := replacedKeys.remoteAddr(req(one)), replacedKeys.remoteAddr(req(two)); a != b {
		t.Fatalf("httprate keyed the two mapped clients %q and %q; this test can no longer tell the two strategies apart", a, b)
	}

	a, _ := KeyByClientIP(req(one))
	b, _ := KeyByClientIP(req(two))
	if a == b {
		t.Fatalf("two mapped IPv4 clients share the key %q", a)
	}
	if a != "203.0.113.9" {
		t.Errorf("mapped client keyed %q, want the dotted quad 203.0.113.9", a)
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
		name     string
		clientIP string
		hostPort string
	}{
		{name: "one IPv4 client", clientIP: "198.51.100.7", hostPort: "198.51.100.7:1234"},
		{name: "one IPv6 client", clientIP: "2001:db8:abcd:1::1", hostPort: "[2001:db8:abcd:1::1]:1234"},
		{name: "different addresses in one /64", clientIP: "2001:db8:abcd:1:2:3:4:5", hostPort: "[2001:db8:abcd:1::1]:1234"},
		{name: "the mapped and dotted spellings of one IPv4 client", clientIP: "::ffff:198.51.100.7", hostPort: "198.51.100.7:1234"},
		{name: "the header entry carries a port", clientIP: "[2001:db8:abcd:1::9]:443", hostPort: "[2001:db8:abcd:1::1]:1234"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if h, ra := viaHeader(tc.clientIP), viaRemoteAddr(tc.hostPort); h != ra {
				t.Errorf("%s keyed %q through the header and %q through RemoteAddr", tc.clientIP, h, ra)
			}
		})
	}
}

// TestKeyByClientIP_IPv6ThroughTheProductionHopCount: the deployed topology
// appends two hops, so the client sits at index len-2. The prefix collapse has to
// hold on that chain, not only on the single-hop one.
func TestKeyByClientIP_IPv6ThroughTheProductionHopCount(t *testing.T) {
	t.Setenv(TrustedProxyHopsEnvVar, "")
	if trustedProxyHops() != 2 {
		t.Fatalf("this test is written for a default of 2 hops, got %d", trustedProxyHops())
	}

	key := func(clientIP string) string {
		t.Helper()
		r := httptest.NewRequest("POST", "/auth/login", nil)
		r.RemoteAddr = "10.0.0.1:4000"
		r.Header.Set("X-Forwarded-For", "2001:db8:9999:9999::ff, "+clientIP+", 10.0.0.1")
		got, _ := KeyByClientIP(r)
		return got
	}

	a, b := key("2001:db8:abcd:7::1"), key("2001:db8:abcd:7:5:6:7:8")
	if a != b {
		t.Errorf("two addresses in one /64 keyed %q and %q behind two hops", a, b)
	}
	if a != "2001:db8:abcd:7::" {
		t.Errorf("key = %q, want the /64 of the entry at index len-2", a)
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
