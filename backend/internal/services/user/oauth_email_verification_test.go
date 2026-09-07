package user

import (
	"testing"

	"github.com/markbates/goth"
	"github.com/stretchr/testify/assert"

	fauxauth "psychic-homily-backend/internal/auth"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

func TestProviderAssertsEmailVerified(t *testing.T) {
	cases := []struct {
		name    string
		rawData map[string]any
		want    bool
	}{
		{"google verified_email true", map[string]any{"verified_email": true}, true},
		{"google verified_email false", map[string]any{"verified_email": false}, false},
		{"oidc email_verified true", map[string]any{"email_verified": true}, true},
		{"oidc email_verified false", map[string]any{"email_verified": false}, false},
		{"string true", map[string]any{"email_verified": "true"}, true},
		{"string false", map[string]any{"email_verified": "false"}, false},
		{"no signal in a populated payload", map[string]any{"login": "octocat", "id": 1}, false},
		{"empty raw data", map[string]any{}, false},
		{"nil raw data", nil, false},
		// A shape nothing sends must not read as an assertion either way.
		{"unparseable value", map[string]any{"email_verified": "yes"}, false},
		{"numeric value", map[string]any{"email_verified": 1}, false},
		{"null value", map[string]any{"email_verified": nil}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := providerAssertsEmailVerified(goth.User{RawData: tc.rawData})
			assert.Equal(t, tc.want, got)
		})
	}
}

// An unreadable value under the first key must not shadow a readable one under
// the second, which is what makes the key list an ordered preference rather
// than a first-match-wins lookup.
func TestProviderAssertsEmailVerified_FallsThroughToSecondKey(t *testing.T) {
	got := providerAssertsEmailVerified(goth.User{RawData: map[string]any{
		"verified_email": "unparseable",
		"email_verified": true,
	}})
	assert.True(t, got)
}

// The faux "google" provider stamps its verification flag under a key it owns.
// This is what holds the two spellings together; without it a rename there
// silently makes the seeded OAuth account unlinkable.
func TestProviderAssertsEmailVerified_ReadsTheFauxProvidersKey(t *testing.T) {
	got := providerAssertsEmailVerified(goth.User{RawData: map[string]any{
		fauxauth.EmailVerifiedRawDataKey: true,
	}})
	assert.True(t, got)
}

// --- integration: the link gate in findOrCreateOAuthUser ---

func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_UnverifiedEmail_RefusesLink() {
	existing := &authm.User{
		Email:         stringPtr("unverified.link@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(existing).Error)

	linked, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "goth-unverified-subject",
		Email:   "unverified.link@example.com",
		RawData: map[string]any{"verified_email": false},
	}, "google")

	suite.Require().Error(err)
	suite.Require().Nil(linked)

	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(apperrors.CodeOAuthLinkRefused, authErr.Code)

	suite.assertNoOAuthAccountFor(existing.ID)
	suite.assertSingleUserForEmail("unverified.link@example.com")
}

// The RawData shape goth's github provider produces: the GET /user body, with
// no verification field under either key. Refused, like any other provider
// that asserts nothing.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_AbsentVerificationSignal_RefusesLink() {
	existing := &authm.User{
		Email:         stringPtr("absent.signal@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(existing).Error)

	linked, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "github-absent-subject",
		Email:   "absent.signal@example.com",
		RawData: map[string]any{"login": "octocat"},
	}, "github")

	suite.Require().Error(err)
	suite.Require().Nil(linked)

	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(apperrors.CodeOAuthLinkRefused, authErr.Code)

	suite.assertNoOAuthAccountFor(existing.ID)
	suite.assertSingleUserForEmail("absent.signal@example.com")
}

func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_VerifiedEmail_LinksAccount() {
	existing := &authm.User{
		Email:         stringPtr("verified.link@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(existing).Error)

	linked, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "goth-verified-subject",
		Email:   "verified.link@example.com",
		RawData: map[string]any{"verified_email": true},
	}, "google")

	suite.Require().NoError(err)
	suite.Require().NotNil(linked)
	suite.Equal(existing.ID, linked.ID)
	suite.Require().Len(linked.OAuthAccounts, 1)
	suite.Equal("goth-verified-subject", linked.OAuthAccounts[0].ProviderUserID)
}

// The gate sits on the link-by-email branch only. A provider identity already
// in oauth_accounts resolves by provider_user_id and never reaches the address
// comparison, so a provider that stops sending the flag cannot break an
// established sign-in.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_AlreadyLinkedAccount_UnaffectedByVerification() {
	existing := &authm.User{
		Email:         stringPtr("already.linked@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(existing).Error)
	suite.Require().NoError(suite.db.Create(&authm.OAuthAccount{
		UserID:         existing.ID,
		Provider:       "google",
		ProviderUserID: "goth-already-linked-subject",
	}).Error)

	resolved, err := suite.userService.FindOrCreateUser(goth.User{
		UserID: "goth-already-linked-subject",
		Email:  "already.linked@example.com",
	}, "google")

	suite.Require().NoError(err)
	suite.Require().NotNil(resolved)
	suite.Equal(existing.ID, resolved.ID)
}

// The other half of the blast radius: an address matching nobody is not a
// takeover, so a first-time signup still creates an account. The account it
// creates is UNVERIFIED, because nothing vouched for the address.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_UnverifiedEmail_NoExistingAccount_CreatesUnverified() {
	created, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "goth-fresh-unverified-subject",
		Email:   "fresh.unverified@example.com",
		RawData: map[string]any{"verified_email": false},
	}, "google")

	suite.Require().NoError(err)
	suite.Require().NotNil(created)
	suite.Equal("fresh.unverified@example.com", *created.Email)
	suite.False(created.EmailVerified, "an address no provider vouched for must not be recorded as verified")
	suite.assertStoredEmailVerified(created.ID, false)
}

// The RawData shape goth's github provider produces carries no signal at all,
// which is not verification either.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_AbsentSignal_NoExistingAccount_CreatesUnverified() {
	created, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "github-fresh-subject",
		Email:   "fresh.github@example.com",
		RawData: map[string]any{"login": "octocat", "id": 1},
	}, "github")

	suite.Require().NoError(err)
	suite.Require().NotNil(created)
	suite.False(created.EmailVerified)
	suite.assertStoredEmailVerified(created.ID, false)
}

func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_VerifiedEmail_NoExistingAccount_CreatesVerified() {
	created, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "goth-fresh-verified-subject",
		Email:   "fresh.verified@example.com",
		RawData: map[string]any{"verified_email": true},
	}, "google")

	suite.Require().NoError(err)
	suite.Require().NotNil(created)
	suite.True(created.EmailVerified)
	suite.assertStoredEmailVerified(created.ID, true)
}

// The faux "google" provider stamps its flag under the key it owns, so a
// create driven by it lands on the same arm a real Google create would.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_FauxProviderShape_CreatesFromItsFlag() {
	unverified, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "faux-unverified-subject",
		Email:   "faux.unverified@example.com",
		RawData: map[string]any{fauxauth.EmailVerifiedRawDataKey: false},
	}, "google")
	suite.Require().NoError(err)
	suite.False(unverified.EmailVerified)

	verified, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "faux-verified-subject",
		Email:   "faux.verified@example.com",
		RawData: map[string]any{fauxauth.EmailVerifiedRawDataKey: true},
	}, "google")
	suite.Require().NoError(err)
	suite.True(verified.EmailVerified)
}

// The squat, end to end. An attacker signs in with a provider that will not
// vouch for an address nobody holds yet, which creates an UNVERIFIED account
// under it. The real owner then signs in with a provider that DOES vouch for
// the same address. The owner must be refused rather than joined into the
// account the attacker holds, and the attacker's account must be left exactly
// as it was.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_SquattedUnverifiedAccount_RefusesVerifiedOwner() {
	squatted, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "goth-squatter-subject",
		Email:   "squatted@example.com",
		RawData: map[string]any{"verified_email": false},
	}, "google")
	suite.Require().NoError(err)
	suite.Require().False(squatted.EmailVerified)

	owner, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "goth-real-owner-subject",
		Email:   "squatted@example.com",
		RawData: map[string]any{"verified_email": true},
	}, "google")

	suite.Require().Error(err)
	suite.Require().Nil(owner)

	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(apperrors.CodeOAuthLinkRefused, authErr.Code)
	// The refusal has to name the way in, or a refused owner has nowhere to go.
	suite.Contains(authErr.UserMessage(), "Settings")

	// The squatter's account keeps its single identity and stays unverified,
	// so nothing was captured and nothing was granted.
	suite.assertSingleUserForEmail("squatted@example.com")
	suite.assertStoredEmailVerified(squatted.ID, false)
	var rows int64
	suite.Require().NoError(
		suite.db.Model(&authm.OAuthAccount{}).Where("user_id = ?", squatted.ID).Count(&rows).Error)
	suite.Equal(int64(1), rows, "the refused owner must not have been added to the squatter's account")
}

// The same rule, reached from the ordinary direction: a password account whose
// owner never clicked the verification link has not proven the mailbox either,
// so a verified provider identity does not join it by address.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_UnverifiedExistingAccount_RefusesVerifiedProvider() {
	existing := &authm.User{
		Email:         stringPtr("never.verified@example.com"),
		IsActive:      true,
		EmailVerified: false,
	}
	suite.Require().NoError(suite.db.Create(existing).Error)

	linked, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "goth-into-unverified-subject",
		Email:   "never.verified@example.com",
		RawData: map[string]any{"verified_email": true},
	}, "google")

	suite.Require().Error(err)
	suite.Require().Nil(linked)

	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(apperrors.CodeOAuthLinkRefused, authErr.Code)

	suite.assertNoOAuthAccountFor(existing.ID)
	suite.assertSingleUserForEmail("never.verified@example.com")
}

// provider_user_id permits the empty string, so a stored row with one would be
// matched by any provider that returns no subject, ahead of the address gate.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_EmptyProviderUserID_Refused() {
	owner := &authm.User{
		Email:         stringPtr("empty.subject.owner@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(owner).Error)
	suite.Require().NoError(suite.db.Create(&authm.OAuthAccount{
		UserID:         owner.ID,
		Provider:       "google",
		ProviderUserID: "",
	}).Error)

	resolved, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "",
		Email:   "someone.else@example.com",
		RawData: map[string]any{"verified_email": true},
	}, "google")

	suite.Require().Error(err)
	suite.Require().Nil(resolved)
	suite.Contains(err.Error(), "no user id")
}

func (suite *UserServiceIntegrationTestSuite) assertNoOAuthAccountFor(userID uint) {
	suite.T().Helper()
	var oauthRows int64
	suite.Require().NoError(
		suite.db.Model(&authm.OAuthAccount{}).Where("user_id = ?", userID).Count(&oauthRows).Error)
	suite.Equal(int64(0), oauthRows, "a refused link must leave no oauth_accounts row")
}

// assertStoredEmailVerified reads the column back rather than trusting the
// struct the service returned, which is what makes a create assertion about
// the row rather than about the value in memory.
func (suite *UserServiceIntegrationTestSuite) assertStoredEmailVerified(userID uint, want bool) {
	suite.T().Helper()
	var stored authm.User
	suite.Require().NoError(suite.db.First(&stored, userID).Error)
	suite.Equal(want, stored.EmailVerified)
}

func (suite *UserServiceIntegrationTestSuite) assertSingleUserForEmail(email string) {
	suite.T().Helper()
	var userRows int64
	suite.Require().NoError(
		suite.db.Model(&authm.User{}).Where(authm.EmailIdentityWhere, email).Count(&userRows).Error)
	suite.Equal(int64(1), userRows, "a refused link must not mint a second account for the mailbox")
}
