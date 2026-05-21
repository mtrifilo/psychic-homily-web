package engagement

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

// PSY-756: tests for the generic scoped unsubscribe HMAC helpers.

func TestComputeScopedUnsubscribeSignature_Deterministic(t *testing.T) {
	sig1 := ComputeScopedUnsubscribeSignature(42, UnsubscribeScopeTierNotifications, testSecret)
	sig2 := ComputeScopedUnsubscribeSignature(42, UnsubscribeScopeTierNotifications, testSecret)
	assert.NotEmpty(t, sig1)
	assert.Equal(t, sig1, sig2, "same inputs should produce the same signature")
	assert.Len(t, sig1, 64, "HMAC-SHA256 hex output should be 64 characters")
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

func TestComputeScopedUnsubscribeSignature_DistinctFromShowReminders(t *testing.T) {
	// The legacy show-reminders helper hardcodes the "show-reminders" scope;
	// the generic helper must produce a different value for other scopes so
	// the domains stay separated.
	legacy := ComputeUnsubscribeSignature(42, testSecret)
	tier := ComputeScopedUnsubscribeSignature(42, UnsubscribeScopeTierNotifications, testSecret)
	assert.NotEqual(t, legacy, tier)
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
