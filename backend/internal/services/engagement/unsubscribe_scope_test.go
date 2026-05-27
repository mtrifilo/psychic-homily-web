package engagement

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Tests for the generic scoped unsubscribe HMAC helpers.

func TestComputeScopedUnsubscribeSignature_Deterministic(t *testing.T) {
	sig1 := ComputeScopedUnsubscribeSignature(42, UnsubscribeScopeTierNotifications, testSecret)
	sig2 := ComputeScopedUnsubscribeSignature(42, UnsubscribeScopeTierNotifications, testSecret)
	assert.NotEmpty(t, sig1)
	assert.Equal(t, sig1, sig2, "same inputs should produce the same signature")
	assert.Len(t, sig1, 64, "HMAC-SHA256 hex output should be 64 characters")
}

func TestComputeScopedUnsubscribeSignature_DifferentUsers(t *testing.T) {
	sig1 := ComputeScopedUnsubscribeSignature(1, UnsubscribeScopeShowReminders, testSecret)
	sig2 := ComputeScopedUnsubscribeSignature(2, UnsubscribeScopeShowReminders, testSecret)
	assert.NotEqual(t, sig1, sig2, "different user IDs should produce different signatures")
}

func TestComputeScopedUnsubscribeSignature_DifferentSecrets(t *testing.T) {
	sig1 := ComputeScopedUnsubscribeSignature(1, UnsubscribeScopeShowReminders, "secret-a")
	sig2 := ComputeScopedUnsubscribeSignature(1, UnsubscribeScopeShowReminders, "secret-b")
	assert.NotEqual(t, sig1, sig2, "different secrets should produce different signatures")
}

func TestComputeScopedUnsubscribeSignature_HexEncoded(t *testing.T) {
	sig := ComputeScopedUnsubscribeSignature(1, UnsubscribeScopeShowReminders, testSecret)
	assert.Len(t, sig, 64, "HMAC-SHA256 hex output should be 64 characters")
	for _, c := range sig {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"signature should be lowercase hex, got char: %c", c)
	}
}

func TestComputeScopedUnsubscribeSignature_ScopeIsolation(t *testing.T) {
	// A signature minted for one scope must not be valid for another, so a
	// leaked tier-notifications link can't unsubscribe edit-notifications.
	tierSig := ComputeScopedUnsubscribeSignature(42, UnsubscribeScopeTierNotifications, testSecret)
	editSig := ComputeScopedUnsubscribeSignature(42, UnsubscribeScopeEditNotifications, testSecret)
	assert.NotEqual(t, tierSig, editSig, "different scopes must produce different signatures")

	assert.False(t, VerifyScopedUnsubscribeSignature(42, UnsubscribeScopeEditNotifications, tierSig, testSecret),
		"a tier-scope signature must not verify under the edit scope")
}

// TestComputeScopedUnsubscribeSignature_MatchesLegacyHelpers is the
// backwards-compat regression gate for the show-reminders, mention, and
// collection-digest scopes. Existing one-click unsubscribe URLs in users'
// inboxes were minted by the per-domain helpers (since deleted) and must keep
// verifying after the cutover. Each case inlines the deleted helper's exact
// payload format and asserts byte-identical hex output against the generic
// helper. Sentinel inputs (NOT production secret rotation) so the assertion
// stays stable across deploys.
func TestComputeScopedUnsubscribeSignature_MatchesLegacyHelpers(t *testing.T) {
	const (
		uid    = uint(42)
		secret = "test-secret-32bytes-pad-padxxx"
	)

	cases := []struct {
		name           string
		scope          string
		legacyTemplate string // exact format string the deleted helper used
	}{
		{"show-reminders", UnsubscribeScopeShowReminders, "unsubscribe:show-reminders:%d"},
		{"mention", UnsubscribeScopeMention, "unsubscribe:mention:%d"},
		{"collection-digest", UnsubscribeScopeCollectionDigest, "unsubscribe:collection-digest:%d"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Inline the deleted helper's body: HMAC-SHA256(secret) over the
			// exact legacy payload, hex-encoded.
			mac := hmac.New(sha256.New, []byte(secret))
			_, _ = fmt.Fprintf(mac, tc.legacyTemplate, uid)
			legacy := hex.EncodeToString(mac.Sum(nil))

			scoped := ComputeScopedUnsubscribeSignature(uid, tc.scope, secret)
			assert.Equal(t, legacy, scoped,
				"scope %q must produce byte-identical hex to the deleted per-domain helper so inbox URLs remain verifiable",
				tc.scope)
		})
	}
}

func TestVerifyScopedUnsubscribeSignature(t *testing.T) {
	sig := ComputeScopedUnsubscribeSignature(7, UnsubscribeScopeEditNotifications, testSecret)
	assert.True(t, VerifyScopedUnsubscribeSignature(7, UnsubscribeScopeEditNotifications, sig, testSecret))
	// Wrong user, wrong sig, wrong secret, empty sig all fail.
	assert.False(t, VerifyScopedUnsubscribeSignature(8, UnsubscribeScopeEditNotifications, sig, testSecret))
	assert.False(t, VerifyScopedUnsubscribeSignature(7, UnsubscribeScopeEditNotifications, "deadbeef", testSecret))
	assert.False(t, VerifyScopedUnsubscribeSignature(7, UnsubscribeScopeEditNotifications, sig, "other-secret"))
	assert.False(t, VerifyScopedUnsubscribeSignature(7, UnsubscribeScopeEditNotifications, "", testSecret))
}

func TestGenerateScopedUnsubscribeURL_Format(t *testing.T) {
	result := GenerateScopedUnsubscribeURL("https://api.example.com", 42, UnsubscribeScopeTierNotifications, testSecret)

	parsed, err := url.Parse(result)
	assert.NoError(t, err)
	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "api.example.com", parsed.Host)
	assert.Equal(t, "/unsubscribe/"+UnsubscribeScopeTierNotifications, parsed.Path)
	assert.Equal(t, "42", parsed.Query().Get("uid"))

	expectedSig := ComputeScopedUnsubscribeSignature(42, UnsubscribeScopeTierNotifications, testSecret)
	assert.Equal(t, expectedSig, parsed.Query().Get("sig"))
}
