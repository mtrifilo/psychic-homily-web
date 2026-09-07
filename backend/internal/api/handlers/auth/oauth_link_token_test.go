package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testTokenSecret = "test-secret-at-least-32-characters-long"

func TestOAuthLinkToken_RoundTrip(t *testing.T) {
	token, err := mintOAuthLinkToken(testTokenSecret, 7)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if !consumeOAuthLinkToken(testTokenSecret, token, 7) {
		t.Fatal("a freshly minted token must spend")
	}
}

// Single use, which is the property that makes a leaked start URL worth
// nothing on a second click.
func TestOAuthLinkToken_SpendsOnce(t *testing.T) {
	token, err := mintOAuthLinkToken(testTokenSecret, 7)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if !consumeOAuthLinkToken(testTokenSecret, token, 7) {
		t.Fatal("first spend must succeed")
	}
	if consumeOAuthLinkToken(testTokenSecret, token, 7) {
		t.Error("a token must not spend twice")
	}
}

// Every binding is load-bearing: drop any one and a token minted in one
// situation becomes spendable in another.
func TestOAuthLinkToken_Bindings(t *testing.T) {
	cases := []struct {
		name          string
		spendSecret   string
		spendUser     uint
		mutateToken   func(string) string
		wantSpendable bool
	}{
		{"correct", testTokenSecret, 7, nil, true},
		{"another account", testTokenSecret, 8, nil, false},
		{"another secret", "a-different-secret-of-sufficient-length", 7, nil, false},
		{
			name: "tampered payload", spendSecret: testTokenSecret, spendUser: 8,
			// Rewrite the subject and keep the old signature: the signature is
			// the only thing stopping a caller minting their own claims.
			mutateToken: func(tok string) string {
				payload, sig, _ := strings.Cut(tok, ".")
				return "8" + payload[1:] + "." + sig
			},
			wantSpendable: false,
		},
		{
			name: "signature only", spendSecret: testTokenSecret, spendUser: 7,
			mutateToken:   func(tok string) string { _, sig, _ := strings.Cut(tok, "."); return sig },
			wantSpendable: false,
		},
		{
			name: "empty", spendSecret: testTokenSecret, spendUser: 7,
			mutateToken:   func(string) string { return "" },
			wantSpendable: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token, err := mintOAuthLinkToken(testTokenSecret, 7)
			if err != nil {
				t.Fatalf("mint: %v", err)
			}
			if tc.mutateToken != nil {
				token = tc.mutateToken(token)
			}
			if got := consumeOAuthLinkToken(tc.spendSecret, token, tc.spendUser); got != tc.wantSpendable {
				t.Errorf("spendable = %v, want %v", got, tc.wantSpendable)
			}
		})
	}
}

// The MAC is domain-separated, so a value the same secret signed for some
// other purpose cannot be presented here as a link token.
func TestOAuthLinkToken_MACIsDomainSeparated(t *testing.T) {
	claims := oauthLinkTokenClaims{
		userID: 7,
		expiry: time.Now().Add(time.Minute),
		nonce:  "undomained-nonce",
	}
	payload := encodeOAuthLinkTokenPayload(claims)

	mac := hmac.New(sha256.New, []byte(testTokenSecret))
	mac.Write([]byte(payload)) // no domain prefix
	undomained := payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if consumeOAuthLinkToken(testTokenSecret, undomained, 7) {
		t.Error("a MAC computed without the domain prefix must not spend")
	}
}

func TestOAuthLinkToken_Expires(t *testing.T) {
	claims := oauthLinkTokenClaims{
		userID: 7,
		expiry: time.Now().Add(-time.Second),
		nonce:  "expired-nonce",
	}
	payload := encodeOAuthLinkTokenPayload(claims)
	token := payload + "." + signOAuthLinkToken(testTokenSecret, payload)

	if consumeOAuthLinkToken(testTokenSecret, token, 7) {
		t.Error("an expired token must not spend")
	}
}

// The origin gate, which both legs of the link run.
//
// The configured frontend is the primary test. Fetch metadata answers "same
// site as this API", and on stage and every preview the frontend is on
// *.vercel.app while the API is on Railway: genuinely cross-site, so a check
// that stopped at the metadata would refuse every Connect outside production.
func TestRequestIsFromOurFrontend(t *testing.T) {
	const frontend = "https://psychichomily.com"

	cases := []struct {
		name     string
		fetch    string
		origin   string
		referer  string
		wantPass bool
	}{
		// The case that was refused before: a cross-site deployment naming us.
		{"cross-site metadata but our Origin", "cross-site", frontend, "", true},
		{"cross-site metadata but our Referer", "cross-site", "", frontend + "/profile?tab=settings", true},
		{"cross-site metadata and another Origin", "cross-site", "https://evil.example", "", false},
		{"cross-site metadata and nothing else", "cross-site", "", "", false},

		{"our Origin, no metadata", "", frontend, "", true},
		{"our Referer, no metadata", "", "", frontend + "/profile", true},
		{"another Origin, no metadata", "", "https://evil.example", "", false},
		{"another Referer, no metadata", "", "", "https://evil.example/x", false},
		{"nothing at all", "", "", "", false},

		// Same-site deployments, where Referer may be stripped entirely.
		{"same-origin metadata", "same-origin", "", "", true},
		{"same-site metadata", "same-site", "", "", true},
		{"user-initiated navigation", "none", "", "", true},

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
