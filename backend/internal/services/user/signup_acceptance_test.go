package user

import (
	"errors"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// An acceptance that names no terms version records no consent, so the
// password sign-up refuses it and creates no account.
func (suite *UserServiceIntegrationTestSuite) TestCreateUserWithPasswordWithLegal_RefusesEmptyAcceptance() {
	for _, tc := range []struct {
		email      string
		acceptance contracts.LegalAcceptance
	}{
		{"zero.acceptance@example.com", contracts.LegalAcceptance{}},
		{"no.terms.version@example.com", contracts.LegalAcceptance{PrivacyVersion: "v1", MinAgeAttested: 16}},
	} {
		suite.Run(tc.email, func() {
			user, err := suite.userService.CreateUserWithPasswordWithLegal(tc.email, "CorrectPassword1!", "Empty", "Acceptance", tc.acceptance)
			suite.Nil(user)
			var authErr *apperrors.AuthError
			suite.Require().True(errors.As(err, &authErr), "got %v", err)
			suite.Equal(apperrors.CodeTermsAcceptanceRequired, authErr.Code)

			var count int64
			suite.Require().NoError(suite.db.Model(&authm.User{}).Where("email = ?", tc.email).Count(&count).Error)
			suite.Zero(count)
		})
	}
}
