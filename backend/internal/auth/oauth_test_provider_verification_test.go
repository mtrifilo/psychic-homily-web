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

// The seeded account at TestProviderEmail is linked by address, and the link
// path only links an address the provider vouches for. Stock faux supplies no
// RawData, so without the default the seeded account is unreachable.
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

// The flag gives the refusal path a driver that runs the real gothic machinery.
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
