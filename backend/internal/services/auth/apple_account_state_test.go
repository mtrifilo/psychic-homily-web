package auth

import (
	"github.com/golang-jwt/jwt/v5"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	usersvc "psychic-homily-backend/internal/services/user"
)

// A returning Apple identity resolves to its account before anything else is
// consulted, so the account's state is the only thing between that identity
// and a session.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_AccountState() {
	states := []struct {
		name      string
		apply     func(userID uint)
		maySignIn bool
	}{
		{name: "active", apply: func(uint) {}, maySignIn: true},
		{name: "deactivated", apply: func(userID uint) {
			s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", userID).Update("is_active", false).Error)
		}},
		{name: "soft-deleted", apply: func(userID uint) {
			s.Require().NoError(usersvc.NewUserService(s.db).SoftDeleteAccount(userID, nil))
		}},
	}

	for _, state := range states {
		s.Run(state.name, func() {
			svc := s.newService()
			claims := &contracts.AppleIdentityTokenClaims{
				Email:            "apple-" + state.name + "@example.com",
				EmailVerified:    true,
				RegisteredClaims: jwt.RegisteredClaims{Subject: "apple-sub-" + state.name},
			}
			created, err := svc.FindOrCreateAppleUser(claims, "", "")
			s.Require().NoError(err)
			state.apply(created.ID)

			user, err := svc.FindOrCreateAppleUser(claims, "", "")

			if state.maySignIn {
				s.Require().NoError(err)
				s.Require().NotNil(user)
				s.Equal(created.ID, user.ID)
				return
			}

			s.Require().Error(err)
			s.Nil(user)
			var authErr *apperrors.AuthError
			s.Require().ErrorAs(err, &authErr)
			s.Equal(apperrors.CodeAccountInactive, authErr.Code)
			s.Equal(apperrors.ErrAccountInactive().UserMessage(), authErr.UserMessage())

			// The refusal creates nothing: the identity still names the one account.
			var identities int64
			s.Require().NoError(s.db.Model(&authm.OAuthAccount{}).
				Where("provider = ? AND provider_user_id = ?", "apple", claims.Subject).
				Count(&identities).Error)
			s.Equal(int64(1), identities)
		})
	}
}
