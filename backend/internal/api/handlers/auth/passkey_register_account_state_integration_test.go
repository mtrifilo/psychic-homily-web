package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-webauthn/webauthn/protocol"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	authsvc "psychic-homily-backend/internal/services/auth"
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

// register/finish refuses a session whose account's sessions were revoked
// after the session check, with the JWT middleware's 401 TOKEN_INVALID and the
// cookie cleared, and stores no credential. The request context carries the
// account as the session check loaded it, with the stamp from before the
// revocation; a handler that judged a freshly loaded account instead would
// store the credential.
func (s *AuthHandlerIntegrationSuite) TestPasskeyRegisterFinishRefusesSessionRevokedSinceTheSessionCheck() {
	h := s.newRealPasskeyHandler()
	email := "register-revoked@example.com"
	user := &authm.User{Email: &email, IsActive: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)
	ctx := testhelpers.CtxWithSessionAuthTime(user, time.Now())

	begin, err := h.BeginRegisterHandler(ctx, &BeginRegisterRequest{})
	s.Require().NoError(err)
	s.Require().True(begin.Body.Success, begin.Body.Message)
	registration, err := softauthn.New(wireOrigin).Register(begin.Body.Options)
	s.Require().NoError(err)
	s.Require().NoError(authsvc.RevokeAllSessions(s.deps.DB, &authm.User{ID: user.ID}, time.Now()))
	input := &FinishRegisterRequest{}
	input.Body.ChallengeID = begin.Body.ChallengeID
	input.Body.Response = registration

	resp, err := h.FinishRegisterHandler(ctx, input)

	s.Nil(resp)
	var refused *middleware.SessionRefusedError
	s.Require().True(errors.As(err, &refused), "expected the session refusal, got %v", err)
	s.Equal(http.StatusUnauthorized, refused.GetStatus())
	s.Equal(autherrors.CodeTokenInvalid, refused.ErrorCode)
	var withHeaders huma.HeadersError
	s.Require().True(errors.As(err, &withHeaders))
	s.Contains(strings.Join(withHeaders.GetHeaders().Values("Set-Cookie"), ";"), "auth_token=;")
	s.Zero(s.credentialCount(user.ID), "a refused registration stores no credential")
}

// A session minted after the account's latest revocation carries its stamp,
// and registers.
func (s *AuthHandlerIntegrationSuite) TestPasskeyRegisterFinishSessionMintedAfterARevocationRegisters() {
	h := s.newRealPasskeyHandler()
	email := "register-after-revocation@example.com"
	user := &authm.User{Email: &email, IsActive: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)
	s.Require().NoError(authsvc.RevokeAllSessions(s.deps.DB, user, time.Now()))

	resp := s.registerPasskey(h, user, softauthn.New(wireOrigin).Register)

	s.True(resp.Body.Success, resp.Body.Message)
	s.Equal(int64(1), s.credentialCount(user.ID))
}
