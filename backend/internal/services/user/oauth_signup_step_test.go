package user

import (
	"errors"
	"time"

	"github.com/markbates/goth"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

// With no consent, a new identity is refused with the error that carries it,
// as the provider asserted it, and that error reads as the consent refusal to
// a caller that only knows AuthError.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUserWithConsent_NilConsentCarriesTheNewIdentity() {
	identity := goth.User{
		UserID:    "google-needs-step",
		Email:     "needs.step@example.com",
		Name:      "Needs Step",
		FirstName: "Needs",
		LastName:  "Step",
		AvatarURL: "https://example.com/needs-step.png",
		RawData:   map[string]any{"verified_email": true},
	}

	_, err := suite.userService.FindOrCreateUserWithConsent(identity, "google", nil)

	var required *contracts.OAuthSignupRequiredError
	suite.Require().True(errors.As(err, &required), "got %v", err)
	suite.Equal(contracts.OAuthIdentity{
		Provider:      "google",
		Subject:       identity.UserID,
		Email:         identity.Email,
		EmailVerified: true,
		Name:          identity.Name,
		FirstName:     identity.FirstName,
		LastName:      identity.LastName,
		AvatarURL:     identity.AvatarURL,
	}, required.Identity)
	var authErr *apperrors.AuthError
	suite.Require().True(errors.As(err, &authErr))
	suite.Equal(apperrors.CodeTermsAcceptanceRequired, authErr.Code)

	var count int64
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("email = ?", identity.Email).Count(&count).Error)
	suite.Zero(count)
}

// A known identity signs in whatever consent carries, nil included, so the
// returning user never reaches the sign-up step.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUserWithConsent_NilConsentSignsInAKnownIdentity() {
	identity := goth.User{UserID: "google-known-nil", Email: "known.nil@example.com", RawData: map[string]any{"verified_email": true}}
	created, err := suite.userService.FindOrCreateUserWithConsent(identity, "google", signupconsent.CurrentOAuthSignupConsent())
	suite.Require().NoError(err)

	found, err := suite.userService.FindOrCreateUserWithConsent(identity, "google", nil)

	suite.Require().NoError(err)
	suite.Equal(created.ID, found.ID)
}

// The date of birth and its attestation time are recorded with the rest of
// the consent, and a display name is taken as given.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUserFromOAuthIdentity_RecordsTheDateOfBirth() {
	consent := signupconsent.CurrentOAuthSignupConsent()
	dateOfBirth := time.Date(1990, time.March, 4, 0, 0, 0, 0, time.UTC)
	consent.DateOfBirth = &dateOfBirth
	consent.MinAgeAttested = 36
	displayName := "Chosen Name"

	user, err := suite.userService.FindOrCreateUserFromOAuthIdentity(contracts.OAuthIdentity{
		Provider: "google", Subject: "google-dob", Email: "dob@example.com", EmailVerified: true,
	}, consent, &displayName)
	suite.Require().NoError(err)

	var stored authm.User
	suite.Require().NoError(suite.db.First(&stored, user.ID).Error)
	suite.Require().NotNil(stored.DateOfBirth)
	suite.Equal("1990-03-04", stored.DateOfBirth.UTC().Format(legal.DateOfBirthLayout))
	suite.Require().NotNil(stored.AgeAttestedAt)
	suite.True(stored.AgeAttestedAt.Equal(*stored.AgeConfirmedAt))
	suite.Equal(36, *stored.MinAgeAttested)
	suite.Require().NotNil(stored.DisplayName)
	suite.Equal(displayName, *stored.DisplayName)
}
