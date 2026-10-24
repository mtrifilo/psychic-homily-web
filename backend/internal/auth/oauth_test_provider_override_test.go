package auth

import (
	"net/url"
	"testing"
)

// completeFauxHandshake runs the clone the way gothic.CompleteUserAuth does:
// the stored session is unmarshalled, authorized with the callback's query,
// and the user fetched from that same session value.
func completeFauxHandshake(t *testing.T, callbackQuery url.Values) (userID, email string) {
	t.Helper()
	p := newTestProvider()
	started, err := p.BeginAuth("state-nonce")
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}
	sess, err := p.UnmarshalSession(started.Marshal())
	if err != nil {
		t.Fatalf("UnmarshalSession: %v", err)
	}
	if _, err := p.FetchUser(sess); err == nil {
		t.Fatal("an unauthorized session fetched a user")
	}
	if _, err := sess.Authorize(p, callbackQuery); err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	user, err := p.FetchUser(sess)
	if err != nil {
		t.Fatalf("FetchUser: %v", err)
	}
	return user.UserID, user.Email
}

func TestTestProvider_CallbackQueryOverridesTheIdentity(t *testing.T) {
	t.Setenv(TestProviderUserIDEnvVar, "env-subject")
	t.Setenv(TestProviderEmailEnvVar, "env@test.local")

	userID, email := completeFauxHandshake(t, url.Values{
		"code":                  {"x"},
		TestProviderUserIDParam: {"query-subject"},
		TestProviderEmailParam:  {"query@test.local"},
	})
	if userID != "query-subject" || email != "query@test.local" {
		t.Errorf("identity = %q/%q, want the callback query's", userID, email)
	}
}

func TestTestProvider_WithoutAQueryOverrideTheEnvironmentAndDefaultsHold(t *testing.T) {
	t.Run("environment", func(t *testing.T) {
		t.Setenv(TestProviderUserIDEnvVar, "env-subject")
		t.Setenv(TestProviderEmailEnvVar, "env@test.local")
		userID, email := completeFauxHandshake(t, url.Values{"code": {"x"}})
		if userID != "env-subject" || email != "env@test.local" {
			t.Errorf("identity = %q/%q, want the environment's", userID, email)
		}
	})
	t.Run("defaults", func(t *testing.T) {
		t.Setenv(TestProviderUserIDEnvVar, "")
		t.Setenv(TestProviderEmailEnvVar, "")
		userID, email := completeFauxHandshake(t, url.Values{"code": {"x"}})
		if userID != TestProviderUserID || email != TestProviderEmail {
			t.Errorf("identity = %q/%q, want the constants", userID, email)
		}
	})
}
