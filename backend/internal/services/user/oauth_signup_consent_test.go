package user

import (
	"errors"
	"testing"
	"time"

	"github.com/markbates/goth"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

func currentOAuthSignupConsent() contracts.OAuthSignupConsent {
	return contracts.OAuthSignupConsent{
		TermsAccepted:  true,
		TermsVersion:   legal.CurrentTermsVersion,
		PrivacyVersion: legal.CurrentPrivacyVersion,
		AcceptedAt:     time.Now().UTC(),
		AgeConfirmed:   true,
		MinAgeAttested: minOAuthSignupAge,
	}
}

// The consent reaches the callback in a cookie the caller can write, so the
// account-creating check refuses any version that is not current on its own.
func TestValidateOAuthSignupConsent_RefusesVersionsThatAreNotCurrent(t *testing.T) {
	for name, mutate := range map[string]func(*contracts.OAuthSignupConsent){
		"terms stale":     func(c *contracts.OAuthSignupConsent) { c.TermsVersion = "2025-01-01" },
		"terms empty":     func(c *contracts.OAuthSignupConsent) { c.TermsVersion = "" },
		"privacy unknown": func(c *contracts.OAuthSignupConsent) { c.PrivacyVersion = "x" },
		"privacy empty":   func(c *contracts.OAuthSignupConsent) { c.PrivacyVersion = "" },
	} {
		t.Run(name, func(t *testing.T) {
			consent := currentOAuthSignupConsent()
			mutate(&consent)
			err := validateOAuthSignupConsent(&consent)
			var authErr *apperrors.AuthError
			if !errors.As(err, &authErr) || authErr.Code != apperrors.CodeTermsAcceptanceRequired {
				t.Fatalf("expected %s, got %v", apperrors.CodeTermsAcceptanceRequired, err)
			}
		})
	}

	consent := currentOAuthSignupConsent()
	if err := validateOAuthSignupConsent(&consent); err != nil {
		t.Fatalf("current consent refused: %v", err)
	}
}

func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUserWithConsent_StaleVersionCreatesNoAccount() {
	consent := currentOAuthSignupConsent()
	consent.PrivacyVersion = "2025-01-01"
	identity := goth.User{
		UserID:  "google-stale-consent",
		Email:   "stale.consent@example.com",
		RawData: map[string]any{"verified_email": true},
	}

	_, err := suite.userService.FindOrCreateUserWithConsent(identity, "google", &consent)
	var authErr *apperrors.AuthError
	suite.Require().True(errors.As(err, &authErr), "got %v", err)
	suite.Equal(apperrors.CodeTermsAcceptanceRequired, authErr.Code)

	var count int64
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("email = ?", identity.Email).Count(&count).Error)
	suite.Zero(count)
}

func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUserWithConsent_RecordsCurrentVersions() {
	consent := currentOAuthSignupConsent()
	identity := goth.User{
		UserID:  "google-current-consent",
		Email:   "current.consent@example.com",
		RawData: map[string]any{"verified_email": true},
	}

	user, err := suite.userService.FindOrCreateUserWithConsent(identity, "google", &consent)
	suite.Require().NoError(err)

	var stored authm.User
	suite.Require().NoError(suite.db.First(&stored, user.ID).Error)
	suite.Require().NotNil(stored.TermsVersion)
	suite.Require().NotNil(stored.PrivacyVersion)
	suite.Equal(legal.CurrentTermsVersion, *stored.TermsVersion)
	suite.Equal(legal.CurrentPrivacyVersion, *stored.PrivacyVersion)
}
