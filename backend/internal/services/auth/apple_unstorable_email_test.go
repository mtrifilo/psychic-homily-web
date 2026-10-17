package auth

import (
	"crypto/rsa"
	"errors"
	"strings"
	"testing"

	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

// unstorableEmailClaims are email claims Postgres rejects as text, keyed by
// why.
var unstorableEmailClaims = map[string]string{
	"NUL":           "no\x00body@example.com",
	"invalid UTF-8": "no\xffbody@example.com",
}

// requireUnstorableEmailRefusal asserts err is the validation refusal for an
// email claim, and that its text, which callers log, does not carry the claim.
func requireUnstorableEmailRefusal(t *testing.T, err error, claim string) {
	t.Helper()
	var refusal *apperrors.AuthError
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v, want an *AuthError refusal", err)
	}
	if refusal.Code != apperrors.CodeValidationFailed {
		t.Fatalf("refusal code = %q, want %q", refusal.Code, apperrors.CodeValidationFailed)
	}
	if strings.Contains(err.Error(), claim) {
		t.Fatalf("refusal text carries the claim: %q", err.Error())
	}
}

// An Apple email claim Postgres rejects as text is refused before any query,
// so the database never sees a parameter it refuses. The database here fails
// every query, which makes a query that does run visible as an error.
func TestFindOrCreateAppleUser_UnstorableEmailClaimIsRefusedWithoutQuerying(t *testing.T) {
	for name, claim := range unstorableEmailClaims {
		t.Run(name, func(t *testing.T) {
			db, queries := testutil.CountQueries(testutil.UnavailableDB(t))
			svc := &AppleAuthService{db: db, config: &config.Config{}, keys: make(map[string]*rsa.PublicKey)}

			user, err := svc.FindOrCreateAppleUser(appleClaims("apple-sub-unstorable", claim), "", "",
				signupconsent.CurrentOAuthSignupConsent())

			if user != nil {
				t.Fatalf("user = %+v, want nil", user)
			}
			requireUnstorableEmailRefusal(t, err, claim)
			if *queries != 0 {
				t.Fatalf("ran %d queries, want 0", *queries)
			}

			// Any other claim reaches the database.
			_, err = svc.FindOrCreateAppleUser(appleClaims("apple-sub-unstorable", "plain@example.com"), "", "",
				signupconsent.CurrentOAuthSignupConsent())
			var refusal *apperrors.AuthError
			if err == nil || errors.As(err, &refusal) {
				t.Fatalf("plain claim on an unavailable database: error = %v, want the lookup's failure", err)
			}
			if *queries == 0 {
				t.Fatal("plain claim ran no query")
			}
		})
	}
}

// Against Postgres, the refusal leaves no account and no Apple identity behind.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_UnstorableEmailClaimCreatesNoAccount() {
	for name, claim := range unstorableEmailClaims {
		s.Run(name, func() {
			var usersBefore, identitiesBefore int64
			s.Require().NoError(s.db.Model(&authm.User{}).Count(&usersBefore).Error)
			s.Require().NoError(s.db.Model(&authm.OAuthAccount{}).Count(&identitiesBefore).Error)

			user, err := s.newService().FindOrCreateAppleUser(appleClaims("apple-sub-unstorable-pg", claim), "", "",
				signupconsent.CurrentOAuthSignupConsent())

			s.Nil(user)
			requireUnstorableEmailRefusal(s.T(), err, claim)
			var usersAfter, identitiesAfter int64
			s.Require().NoError(s.db.Model(&authm.User{}).Count(&usersAfter).Error)
			s.Require().NoError(s.db.Model(&authm.OAuthAccount{}).Count(&identitiesAfter).Error)
			s.Equal(usersBefore, usersAfter)
			s.Equal(identitiesBefore, identitiesAfter)
		})
	}
}
