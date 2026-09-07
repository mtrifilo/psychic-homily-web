package auth

import (
	"testing"

	"github.com/markbates/goth"
)

// authorizedFauxUser drives the clone's FetchUser through faux's
// begin->authorize gate, which FetchUser requires an AccessToken from.
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

// The create path records email_verified from what the provider vouches for,
// and stock faux supplies no RawData, so the clone has to stamp the flag
// itself.
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

// The identity overrides, which is what lets a person drive by hand the
// sequences that need a SECOND address or a SECOND subject: the constants
// alone can produce neither.
func TestTestProvider_FetchUser_IdentityOverrides(t *testing.T) {
	t.Setenv(TestProviderEmailEnvVar, "squatter@example.com")
	t.Setenv(TestProviderUserIDEnvVar, "squatter-subject")

	user := authorizedFauxUser(t)

	if user.Email != "squatter@example.com" {
		t.Errorf("Email = %q, want the override", user.Email)
	}
	if user.UserID != "squatter-subject" {
		t.Errorf("UserID = %q, want the override", user.UserID)
	}
}

// Empty is unset. An empty address makes the find-or-create path key on "",
// and an empty subject is refused outright, so neither is a value a caller
// could have meant.
func TestTestProvider_FetchUser_EmptyOverridesFallBackToTheConstants(t *testing.T) {
	t.Setenv(TestProviderEmailEnvVar, "")
	t.Setenv(TestProviderUserIDEnvVar, "")

	user := authorizedFauxUser(t)

	if user.Email != TestProviderEmail {
		t.Errorf("Email = %q, want %q", user.Email, TestProviderEmail)
	}
	if user.UserID != TestProviderUserID {
		t.Errorf("UserID = %q, want %q", user.UserID, TestProviderUserID)
	}
}

// Under the flag the clone reports unverified. Nothing else in the tree sets
// the flag; a person sets it and restarts the backend to reproduce a refusal.
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
