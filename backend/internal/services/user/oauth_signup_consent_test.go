package user

import (
	"errors"
	"time"

	"github.com/markbates/goth"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUserWithConsent_StaleVersionCreatesNoAccount() {
	consent := signupconsent.CurrentOAuthSignupConsent()
	consent.PrivacyVersion = "2025-01-01"
	identity := goth.User{
		UserID:  "google-stale-consent",
		Email:   "stale.consent@example.com",
		RawData: map[string]any{"verified_email": true},
	}

	_, err := suite.userService.FindOrCreateUserWithConsent(identity, "google", consent)
	var authErr *apperrors.AuthError
	suite.Require().True(errors.As(err, &authErr), "got %v", err)
	suite.Equal(apperrors.CodeTermsAcceptanceRequired, authErr.Code)

	var count int64
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("email = ?", identity.Email).Count(&count).Error)
	suite.Zero(count)
}

func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUserWithConsent_RecordsCurrentVersions() {
	consent := signupconsent.CurrentOAuthSignupConsent()
	identity := goth.User{
		UserID:  "google-current-consent",
		Email:   "current.consent@example.com",
		RawData: map[string]any{"verified_email": true},
	}

	user, err := suite.userService.FindOrCreateUserWithConsent(identity, "google", consent)
	suite.Require().NoError(err)

	var stored authm.User
	suite.Require().NoError(suite.db.First(&stored, user.ID).Error)
	suite.Require().NotNil(stored.TermsVersion)
	suite.Require().NotNil(stored.PrivacyVersion)
	suite.Equal(legal.CurrentTermsVersion, *stored.TermsVersion)
	suite.Equal(legal.CurrentPrivacyVersion, *stored.PrivacyVersion)

	// The age evidence is recorded too, at the instant the terms were accepted.
	suite.Require().NotNil(stored.TermsAcceptedAt)
	suite.Require().NotNil(stored.AgeConfirmedAt)
	suite.Require().NotNil(stored.MinAgeAttested)
	suite.WithinDuration(consent.AcceptedAt, *stored.TermsAcceptedAt, time.Millisecond)
	suite.True(stored.AgeConfirmedAt.Equal(*stored.TermsAcceptedAt))
	suite.Equal(legal.MinSignupAge, *stored.MinAgeAttested)
}
