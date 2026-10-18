package auth

import (
	"github.com/go-webauthn/webauthn/protocol"

	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil/softauthn"
)

// register/finish refuses an account closed after its session was checked,
// with ACCOUNT_INACTIVE and the inactive copy, and stores no credential. The
// request context still carries the account as the session check loaded it,
// live.
func (s *AuthHandlerIntegrationSuite) TestPasskeyRegisterFinishRefusesAccountClosedSinceTheSessionCheck() {
	closures := []struct {
		name         string
		closeAccount func(userID uint)
	}{
		{"deactivated", func(userID uint) {
			s.Require().NoError(s.deps.DB.Model(&authm.User{}).Where("id = ?", userID).Update("is_active", false).Error)
		}},
		{"soft-deleted", func(userID uint) {
			s.Require().NoError(usersvc.NewUserService(s.deps.DB).SoftDeleteAccount(userID, nil))
		}},
	}
	h := s.newRealPasskeyHandler()
	inactive := autherrors.ErrAccountInactive()

	for _, closure := range closures {
		name, closeAccount := closure.name, closure.closeAccount
		s.Run(name, func() {
			email := "register-closed-" + name + "@example.com"
			user := &authm.User{Email: &email, IsActive: true}
			s.Require().NoError(s.deps.DB.Create(user).Error)
			device := softauthn.New(wireOrigin)

			resp := s.registerPasskey(h, user, func(options *protocol.CredentialCreation) ([]byte, error) {
				closeAccount(user.ID)
				return device.Register(options)
			})

			s.False(resp.Body.Success)
			s.Equal(autherrors.CodeAccountInactive, resp.Body.ErrorCode)
			s.Equal(inactive.UserMessage(), resp.Body.Message)
			s.Nil(resp.Body.Credential)
			s.Zero(s.credentialCount(user.ID), "a refused registration stores no credential")
		})
	}
}
