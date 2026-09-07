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

// TestTestProvider_FetchUser_ReportsVerifiedEmailByDefault: the E2E seed
// pre-creates an account at TestProviderEmail, and the link-by-email path only
// links an address the provider vouches for. Stock faux supplies no RawData at
// all, so without this the seeded account could never be signed into.
func TestTestProvider_FetchUser_ReportsVerifiedEmailByDefault(t *testing.T) {
	t.Setenv(TestProviderUnverifiedEmailEnvVar, "")

	user := authorizedFauxUser(t)

	verified, ok := user.RawData[emailVerifiedRawDataKey].(bool)
	if !ok {
		t.Fatalf("RawData[%q] = %#v, want a bool", emailVerifiedRawDataKey, user.RawData[emailVerifiedRawDataKey])
	}
	if !verified {
		t.Errorf("RawData[%q] = false, want true by default", emailVerifiedRawDataKey)
	}
}

// TestTestProvider_FetchUser_ReportsUnverifiedEmailWhenFlagged gives the
// refusal path a driver that goes through the real gothic machinery.
func TestTestProvider_FetchUser_ReportsUnverifiedEmailWhenFlagged(t *testing.T) {
	t.Setenv(TestProviderUnverifiedEmailEnvVar, "1")

	user := authorizedFauxUser(t)

	verified, ok := user.RawData[emailVerifiedRawDataKey].(bool)
	if !ok {
		t.Fatalf("RawData[%q] = %#v, want a bool", emailVerifiedRawDataKey, user.RawData[emailVerifiedRawDataKey])
	}
	if verified {
		t.Errorf("RawData[%q] = true, want false under %s=1", emailVerifiedRawDataKey, TestProviderUnverifiedEmailEnvVar)
	}
}

// The key the clone writes must be one the link path actually reads. Spelled
// out here because the clone answers as "google", and goth's google provider
// reads oauth2/v2/userinfo, whose field is verified_email rather than the OIDC
// email_verified.
func TestTestProvider_EmailVerifiedKeyMatchesGoogleUserinfoField(t *testing.T) {
	if emailVerifiedRawDataKey != "verified_email" {
		t.Errorf("emailVerifiedRawDataKey = %q, want verified_email", emailVerifiedRawDataKey)
	}
}
