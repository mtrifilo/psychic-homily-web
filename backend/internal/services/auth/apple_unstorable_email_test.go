package auth

import (
	"crypto/rsa"
	"errors"
	"testing"

	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/testutil/providerclaim"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

// An Apple email claim Postgres rejects as text is refused before any query,
// so the database never sees a parameter it refuses. The database here fails
// every query, which makes a query that does run visible as an error.
func TestFindOrCreateAppleUser_UnstorableEmailClaimIsRefusedWithoutQuerying(t *testing.T) {
	db, queries := testutil.CountQueries(testutil.UnavailableDB(t))
	svc := &AppleAuthService{db: db, config: &config.Config{}, keys: make(map[string]*rsa.PublicKey)}
	findOrCreate := func(email string) (*authm.User, error) {
		return svc.FindOrCreateAppleUser(appleClaims("apple-sub-unstorable", email), "", "",
			signupconsent.CurrentOAuthSignupConsent())
	}

	for name, claim := range providerclaim.UnstorableEmails {
		t.Run(name, func(t *testing.T) {
			user, err := findOrCreate(claim)

			if user != nil {
				t.Fatalf("user = %+v, want nil", user)
			}
			providerclaim.RequireUnstorableEmailRefusal(t, err, claim)
			if *queries != 0 {
				t.Fatalf("ran %d queries, want 0", *queries)
			}
		})
	}

	// Any other claim reaches the database.
	_, err := findOrCreate("plain@example.com")
	var refusal *apperrors.AuthError
	if err == nil || errors.As(err, &refusal) {
		t.Fatalf("plain claim on an unavailable database: error = %v, want the lookup's failure", err)
	}
	if *queries == 0 {
		t.Fatal("plain claim ran no query")
	}
}

// Against Postgres, the refusal leaves no account and no Apple identity behind.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_UnstorableEmailClaimCreatesNoAccount() {
	for name, claim := range providerclaim.UnstorableEmails {
		s.Run(name, func() {
			usersBefore, identitiesBefore := s.countRows(&authm.User{}), s.countRows(&authm.OAuthAccount{})

			user, err := s.newService().FindOrCreateAppleUser(appleClaims("apple-sub-unstorable-pg", claim), "", "",
				signupconsent.CurrentOAuthSignupConsent())

			s.Nil(user)
			providerclaim.RequireUnstorableEmailRefusal(s.T(), err, claim)
			s.Equal(usersBefore, s.countRows(&authm.User{}))
			s.Equal(identitiesBefore, s.countRows(&authm.OAuthAccount{}))
		})
	}
}
