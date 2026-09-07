package auth

import (
	"testing"

	"github.com/markbates/goth"
)

// authorizedFauxUser drives the clone's FetchUser through faux's
// begin->authorize gate, the same order gothic.CompleteUserAuth uses.
func authorizedFauxUser(t *testing.T) goth.User {
	t.Helper()
	p := newTestProvider()
	sess, err := p.BeginAuth("verification-state")
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}
	if _, err := sess.Authorize(p, goth.Params(nil)); err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	user, err := p.FetchUser(sess)
	if err != nil {
		t.Fatalf("FetchUser: %v", err)
	}
	return user
}

// The link-by-email path only links an address the provider vouches for, and
// stock faux supplies no RawData, so the clone has to stamp the flag itself.
func TestTestProvider_FetchUser_ReportsVerifiedEmailByDefault(t *testing.T) {
	t.Setenv(TestProviderUnverifiedEmailEnvVar, "")

	user := authorizedFauxUser(t)

	verified, ok := user.RawData[EmailVerifiedRawDataKey].(bool)
	if !ok {
		t.Fatalf("RawData[%q] = %#v, want a bool", EmailVerifiedRawDataKey, user.RawData[EmailVerifiedRawDataKey])
	}
	if !verified {
		t.Errorf("RawData[%q] = false, want true by default", EmailVerifiedRawDataKey)
	}
}

// Under the flag the clone reports unverified, which is what drives the link
// refusal through the real gothic begin-authorize-callback sequence.
func TestTestProvider_FetchUser_ReportsUnverifiedEmailWhenFlagged(t *testing.T) {
	t.Setenv(TestProviderUnverifiedEmailEnvVar, "1")

	user := authorizedFauxUser(t)

	verified, ok := user.RawData[EmailVerifiedRawDataKey].(bool)
	if !ok {
		t.Fatalf("RawData[%q] = %#v, want a bool", EmailVerifiedRawDataKey, user.RawData[EmailVerifiedRawDataKey])
	}
	if verified {
		t.Errorf("RawData[%q] = true, want false under %s=1", EmailVerifiedRawDataKey, TestProviderUnverifiedEmailEnvVar)
	}
}
