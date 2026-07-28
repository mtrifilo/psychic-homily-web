package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClientIPFromForwardedFor_SpoofResistance is the security-critical case.
//
// X-Forwarded-For is client-supplied up to the point the trusted proxy appends
// what it actually observed. Reading from the LEFT (which httprate.KeyByRealIP
// does) lets an attacker mint a fresh rate-limit bucket per request simply by
// varying a header — the exact failure PSY-1608 fixes, but deliberate and
// unbounded. Reading from the RIGHT, offset by the trusted hop count, cannot be
// influenced by the caller.
func TestClientIPFromForwardedFor_SpoofResistance(t *testing.T) {
	const realClient = "203.0.113.9"

	cases := []struct {
		name   string
		header string
		hops   int
		want   string
	}{
		{
			name:   "single trusted proxy, no spoofing",
			header: realClient,
			hops:   1,
			want:   realClient,
		},
		{
			name:   "attacker prepends a fake entry — the appended real one still wins",
			header: "1.2.3.4, " + realClient,
			hops:   1,
			want:   realClient,
		},
		{
			name:   "attacker prepends MANY fake entries",
			header: "1.2.3.4, 5.6.7.8, 9.10.11.12, " + realClient,
			hops:   1,
			want:   realClient,
		},
		{
			name:   "attacker varies the spoofed prefix — key must not change",
			header: "99.99.99.99, " + realClient,
			hops:   1,
			want:   realClient,
		},
		{
			name:   "two trusted hops reads one further left",
			header: "1.2.3.4, " + realClient + ", 10.0.0.1",
			hops:   2,
			want:   realClient,
		},
		{
			name:   "chain shorter than the trusted hop count is NOT trusted",
			header: realClient,
			hops:   2,
			want:   "",
		},
		{
			name:   "proxy appended host:port",
			header: realClient + ":54321",
			hops:   1,
			want:   realClient,
		},
		{
			name:   "IPv6",
			header: "1.2.3.4, 2001:db8::1",
			hops:   1,
			want:   "2001:db8::1",
		},
		{name: "empty header", header: "", hops: 1, want: ""},
		{name: "garbage is rejected, not passed through", header: "not-an-ip", hops: 1, want: ""},
		{name: "empty final entry", header: "1.2.3.4, ", hops: 1, want: ""},
		{name: "zero trusted hops trusts nothing", header: realClient, hops: 0, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clientIPFromForwardedFor(tc.header, tc.hops); got != tc.want {
				t.Errorf("clientIPFromForwardedFor(%q, %d) = %q, want %q",
					tc.header, tc.hops, got, tc.want)
			}
		})
	}
}

// TestKeyByClientIP_DistinguishesClients pins the behaviour that was broken in
// production: two different clients arriving through the SAME proxy must land in
// different buckets. Before PSY-1608 the key was the proxy's address, so they
// shared one — meaning one abuser could exhaust the budget for everyone else
// routed through that edge node.
func TestKeyByClientIP_DistinguishesClients(t *testing.T) {
	newReq := func(xff string) *http.Request {
		r := httptest.NewRequest("POST", "/auth/login", nil)
		r.RemoteAddr = "10.0.0.1:4000" // same proxy for both callers
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return r
	}

	a, err := KeyByClientIP(newReq("198.51.100.1"))
	if err != nil {
		t.Fatalf("KeyByClientIP: %v", err)
	}
	b, err := KeyByClientIP(newReq("198.51.100.2"))
	if err != nil {
		t.Fatalf("KeyByClientIP: %v", err)
	}

	if a == b {
		t.Fatalf("two clients behind one proxy share a bucket (%q) — the per-edge-node bug is back", a)
	}

	// And the same client is stable across requests, or the limit never accrues.
	again, _ := KeyByClientIP(newReq("198.51.100.1"))
	if again != a {
		t.Errorf("same client produced different keys %q and %q; the limit would never accumulate", a, again)
	}
}

// TestKeyByClientIP_FallsBackToRemoteAddr: with no X-Forwarded-For (direct
// connection, local dev, a proxy that does not set it) the key degrades to the
// previous behaviour rather than to no limiting at all.
func TestKeyByClientIP_FallsBackToRemoteAddr(t *testing.T) {
	r := httptest.NewRequest("POST", "/auth/login", nil)
	r.RemoteAddr = "198.51.100.7:1234"

	got, err := KeyByClientIP(r)
	if err != nil {
		t.Fatalf("KeyByClientIP: %v", err)
	}
	if got != "198.51.100.7" {
		t.Errorf("fallback key = %q, want the RemoteAddr host 198.51.100.7", got)
	}
}

// TestKeyByClientIP_IgnoresClientSuppliedIdentityHeaders guards the specific
// trap in httprate.KeyByRealIP, which honours these unconditionally. They are
// set by the caller and must not influence the bucket.
func TestKeyByClientIP_IgnoresClientSuppliedIdentityHeaders(t *testing.T) {
	base := func() *http.Request {
		r := httptest.NewRequest("POST", "/auth/login", nil)
		r.RemoteAddr = "10.0.0.1:4000"
		r.Header.Set("X-Forwarded-For", "198.51.100.5")
		return r
	}

	want, _ := KeyByClientIP(base())

	for _, h := range []string{"True-Client-IP", "X-Real-IP"} {
		r := base()
		r.Header.Set(h, "66.66.66.66")
		got, _ := KeyByClientIP(r)
		if got != want {
			t.Errorf("%s changed the bucket key (%q -> %q); it is client-supplied and must be ignored", h, want, got)
		}
	}
}
