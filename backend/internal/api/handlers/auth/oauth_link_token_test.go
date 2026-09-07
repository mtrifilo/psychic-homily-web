package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testTokenSecret = "test-secret-at-least-32-characters-long"
	testTokenOrigin = "https://psychichomily.com"
)

func TestOAuthLinkToken_RoundTrip(t *testing.T) {
	token, err := mintOAuthLinkToken(testTokenSecret, 7, testTokenOrigin)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if !consumeOAuthLinkToken(testTokenSecret, token, 7, testTokenOrigin) {
		t.Fatal("a freshly minted token must spend")
	}
}

// Single use, which is the property that makes a leaked start URL worth
// nothing on a second click.
func TestOAuthLinkToken_SpendsOnce(t *testing.T) {
	token, err := mintOAuthLinkToken(testTokenSecret, 7, testTokenOrigin)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if !consumeOAuthLinkToken(testTokenSecret, token, 7, testTokenOrigin) {
		t.Fatal("first spend must succeed")
	}
	if consumeOAuthLinkToken(testTokenSecret, token, 7, testTokenOrigin) {
		t.Error("a token must not spend twice")
	}
}

// Every binding is load-bearing: drop any one of them and a token minted in
// one situation becomes spendable in another.
func TestOAuthLinkToken_Bindings(t *testing.T) {
	cases := []struct {
		name          string
		spendSecret   string
		spendUser     uint
		spendOrigin   string
		mutateToken   func(string) string
		wantSpendable bool
	}{
		{"correct", testTokenSecret, 7, testTokenOrigin, nil, true},
		{"another account", testTokenSecret, 8, testTokenOrigin, nil, false},
		{"another origin", testTokenSecret, 7, "https://evil.example", nil, false},
		{"another secret", "a-different-secret-of-sufficient-length", 7, testTokenOrigin, nil, false},
		{
			name: "tampered payload", spendSecret: testTokenSecret, spendUser: 8, spendOrigin: testTokenOrigin,
			// Rewrite the subject and keep the old signature: the signature is
			// the only thing stopping a caller minting their own claims.
			mutateToken: func(tok string) string {
				payload, sig, _ := strings.Cut(tok, ".")
				return "8" + payload[1:] + "." + sig
			},
			wantSpendable: false,
		},
		{
			name: "signature only", spendSecret: testTokenSecret, spendUser: 7, spendOrigin: testTokenOrigin,
			mutateToken:   func(tok string) string { _, sig, _ := strings.Cut(tok, "."); return sig },
			wantSpendable: false,
		},
		{
			name: "empty", spendSecret: testTokenSecret, spendUser: 7, spendOrigin: testTokenOrigin,
			mutateToken:   func(string) string { return "" },
			wantSpendable: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token, err := mintOAuthLinkToken(testTokenSecret, 7, testTokenOrigin)
			if err != nil {
				t.Fatalf("mint: %v", err)
			}
			if tc.mutateToken != nil {
				token = tc.mutateToken(token)
			}
			got := consumeOAuthLinkToken(tc.spendSecret, token, tc.spendUser, tc.spendOrigin)
			if got != tc.wantSpendable {
				t.Errorf("spendable = %v, want %v", got, tc.wantSpendable)
			}
		})
	}
}

func TestOAuthLinkToken_Expires(t *testing.T) {
	claims := oauthLinkTokenClaims{
		userID: 7,
		origin: testTokenOrigin,
		expiry: time.Now().Add(-time.Second),
		nonce:  "expired-nonce",
	}
	payload := encodeOAuthLinkTokenPayload(claims)
	token := payload + "." + signOAuthLinkToken(testTokenSecret, payload)

	if consumeOAuthLinkToken(testTokenSecret, token, 7, testTokenOrigin) {
		t.Error("an expired token must not spend")
	}
}

// An origin carrying the field separator must not be able to shift the
// boundaries and present as different claims.
func TestOAuthLinkToken_OriginCannotForgeFields(t *testing.T) {
	hostile := "https://evil.example|9999999999|nonce|"
	token, err := mintOAuthLinkToken(testTokenSecret, 7, hostile)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if !consumeOAuthLinkToken(testTokenSecret, token, 7, hostile) {
		t.Error("the escaped origin must round-trip exactly")
	}
	if consumeOAuthLinkToken(testTokenSecret, token, 7, "https://evil.example") {
		t.Error("a prefix of the origin must not spend")
	}
}

// The start's origin gate. Fail-closed is the whole point: a request that says
// nothing about where it came from is refused, not admitted.
func TestRequestIsFromOurFrontend(t *testing.T) {
	const frontend = "https://psychichomily.com"

	cases := []struct {
		name     string
		fetch    string
		origin   string
		referer  string
		wantPass bool
	}{
		{"same-origin metadata", "same-origin", "", "", true},
		{"same-site metadata", "same-site", "", "", true},
		{"user-initiated navigation", "none", "", "", true},
		{"declared cross-site", "cross-site", frontend, "", false},
		{"cross-origin", "cross-origin", "", "", false},
		{"no metadata, our origin", "", frontend, "", true},
		{"no metadata, another origin", "", "https://evil.example", "", false},
		{"no metadata, our referer", "", "", frontend + "/profile?tab=settings", true},
		{"no metadata, another referer", "", "", "https://evil.example/x", false},
		{"no metadata, nothing at all", "", "", "", false},
		{"opaque origin", "", "null", "", false},
		{"scheme mismatch", "", "http://psychichomily.com", "", false},
		// A host that merely ends with ours is a different host.
		{"suffix host", "", "https://notpsychichomily.com", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/auth/link/google", nil)
			if tc.fetch != "" {
				req.Header.Set("Sec-Fetch-Site", tc.fetch)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}
			if got := requestIsFromOurFrontend(req, frontend); got != tc.wantPass {
				t.Errorf("requestIsFromOurFrontend = %v, want %v", got, tc.wantPass)
			}
		})
	}
}
