package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// A returning Apple identity resolves to its account before anything else is
// consulted, so the account's state is the only thing standing between that
// identity and a session.

func appleClaimsFor(subject, address string) *contracts.AppleIdentityTokenClaims {
	return &contracts.AppleIdentityTokenClaims{
		Email:            address,
		EmailVerified:    true,
		RegisteredClaims: jwt.RegisteredClaims{Subject: subject},
	}
}

func (s *AppleAuthIntegrationTestSuite) assertAccountInactiveRefusal(user *authm.User, err error) {
	s.T().Helper()
	s.Require().Error(err)
	s.Nil(user)

	var authErr *apperrors.AuthError
	s.Require().ErrorAs(err, &authErr)
	s.Equal(apperrors.CodeAccountInactive, authErr.Code)
	s.Equal(apperrors.ErrAccountInactive().UserMessage(), authErr.UserMessage())
}

func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_DeactivatedAccount_Refused() {
	svc := s.newService()
	claims := appleClaimsFor("apple-sub-deactivated", "apple-deactivated@example.com")

	created, err := svc.FindOrCreateAppleUser(claims, "", "")
	s.Require().NoError(err)
	s.Require().NoError(
		s.db.Model(&authm.User{}).Where("id = ?", created.ID).Update("is_active", false).Error)

	user, err := svc.FindOrCreateAppleUser(claims, "", "")
	s.assertAccountInactiveRefusal(user, err)
}

func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_SoftDeletedAccount_Refused() {
	svc := s.newService()
	claims := appleClaimsFor("apple-sub-soft-deleted", "apple-soft-deleted@example.com")

	created, err := svc.FindOrCreateAppleUser(claims, "", "")
	s.Require().NoError(err)
	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", created.ID).Updates(map[string]any{
		"is_active":  false,
		"deleted_at": time.Now(),
	}).Error)

	user, err := svc.FindOrCreateAppleUser(claims, "", "")
	s.assertAccountInactiveRefusal(user, err)

	// The refusal creates nothing: the identity still names the one account.
	var identities int64
	s.Require().NoError(s.db.Model(&authm.OAuthAccount{}).
		Where("provider = ? AND provider_user_id = ?", "apple", "apple-sub-soft-deleted").
		Count(&identities).Error)
	s.Equal(int64(1), identities)
}

func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_ActiveAccount_SignsIn() {
	svc := s.newService()
	claims := appleClaimsFor("apple-sub-active", "apple-active@example.com")

	created, err := svc.FindOrCreateAppleUser(claims, "", "")
	s.Require().NoError(err)

	user, err := svc.FindOrCreateAppleUser(claims, "", "")
	s.Require().NoError(err)
	s.Require().NotNil(user)
	s.Equal(created.ID, user.ID)
}
