package user

import (
	"errors"
	"testing"

	"github.com/markbates/goth"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/testutil/providerclaim"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

func gothIdentity(subject, email string) goth.User {
	return goth.User{UserID: subject, Email: email, RawData: map[string]any{"verified_email": true}}
}

// A provider email claim Postgres rejects as text is refused before any query,
// so the database never sees a parameter it refuses. The database here fails
// every query, which makes a query that does run visible as an error.
func TestFindOrCreateUserWithConsent_UnstorableEmailClaimIsRefusedWithoutQuerying(t *testing.T) {
	db, queries := testutil.CountQueries(testutil.UnavailableDB(t))
	svc := &UserService{db: db}
	findOrCreate := func(email string) (*authm.User, error) {
		return svc.FindOrCreateUserWithConsent(gothIdentity("google-sub-unstorable", email), "google",
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

// Against Postgres, the refusal leaves no account and no provider identity
// behind.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUserWithConsent_UnstorableEmailClaimCreatesNoAccount() {
	countRows := func(model any) int64 {
		var n int64
		suite.Require().NoError(suite.db.Model(model).Count(&n).Error)
		return n
	}
	for name, claim := range providerclaim.UnstorableEmails {
		suite.Run(name, func() {
			usersBefore, identitiesBefore := countRows(&authm.User{}), countRows(&authm.OAuthAccount{})

			user, err := suite.userService.FindOrCreateUserWithConsent(gothIdentity("google-sub-unstorable-pg", claim), "google",
				signupconsent.CurrentOAuthSignupConsent())

			suite.Nil(user)
			providerclaim.RequireUnstorableEmailRefusal(suite.T(), err, claim)
			suite.Equal(usersBefore, countRows(&authm.User{}))
			suite.Equal(identitiesBefore, countRows(&authm.OAuthAccount{}))
		})
	}
}
