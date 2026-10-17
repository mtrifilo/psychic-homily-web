package user

import (
	"errors"
	"strings"
	"testing"

	"github.com/markbates/goth"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

// unstorableEmailClaims are provider email claims Postgres rejects as text,
// keyed by why.
var unstorableEmailClaims = map[string]string{
	"NUL":           "no\x00body@example.com",
	"invalid UTF-8": "no\xffbody@example.com",
}

func gothIdentity(subject, email string) goth.User {
	return goth.User{UserID: subject, Email: email, RawData: map[string]any{"verified_email": true}}
}

// requireUnstorableEmailRefusal asserts err is the validation refusal for a
// provider email claim, and that its text, which callers log, does not carry
// the claim.
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

// A provider email claim Postgres rejects as text is refused before any query,
// so the database never sees a parameter it refuses. The database here fails
// every query, which makes a query that does run visible as an error.
func TestFindOrCreateUserWithConsent_UnstorableEmailClaimIsRefusedWithoutQuerying(t *testing.T) {
	for name, claim := range unstorableEmailClaims {
		t.Run(name, func(t *testing.T) {
			db, queries := testutil.CountQueries(testutil.UnavailableDB(t))
			svc := &UserService{db: db}

			user, err := svc.FindOrCreateUserWithConsent(gothIdentity("google-sub-unstorable", claim), "google",
				signupconsent.CurrentOAuthSignupConsent())

			if user != nil {
				t.Fatalf("user = %+v, want nil", user)
			}
			requireUnstorableEmailRefusal(t, err, claim)
			if *queries != 0 {
				t.Fatalf("ran %d queries, want 0", *queries)
			}

			// Any other claim reaches the database.
			_, err = svc.FindOrCreateUserWithConsent(gothIdentity("google-sub-unstorable", "plain@example.com"), "google",
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

// Against Postgres, the refusal leaves no account and no provider identity
// behind.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUserWithConsent_UnstorableEmailClaimCreatesNoAccount() {
	for name, claim := range unstorableEmailClaims {
		suite.Run(name, func() {
			var usersBefore, identitiesBefore int64
			suite.Require().NoError(suite.db.Model(&authm.User{}).Count(&usersBefore).Error)
			suite.Require().NoError(suite.db.Model(&authm.OAuthAccount{}).Count(&identitiesBefore).Error)

			user, err := suite.userService.FindOrCreateUserWithConsent(gothIdentity("google-sub-unstorable-pg", claim), "google",
				signupconsent.CurrentOAuthSignupConsent())

			suite.Nil(user)
			requireUnstorableEmailRefusal(suite.T(), err, claim)
			var usersAfter, identitiesAfter int64
			suite.Require().NoError(suite.db.Model(&authm.User{}).Count(&usersAfter).Error)
			suite.Require().NoError(suite.db.Model(&authm.OAuthAccount{}).Count(&identitiesAfter).Error)
			suite.Equal(usersBefore, usersAfter)
			suite.Equal(identitiesBefore, identitiesAfter)
		})
	}
}
