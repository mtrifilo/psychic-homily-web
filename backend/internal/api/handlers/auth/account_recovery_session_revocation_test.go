package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	servicesshared "psychic-homily-backend/internal/services/shared"
)

// Deleting an account retires every session it held: after the owner recovers
// it, by password or by the emailed token, a session minted before the
// deletion is refused on GET /auth/profile with 401 TOKEN_INVALID, and the
// session the recovery minted is honored.

const recoverySessionPassword = "strong-password-123!"

// profileThroughTheMiddleware gets /auth/profile carrying session as the auth
// cookie through the strict JWT middleware the route mounts and the real
// handler, around the real services.
func (s *AuthHandlerIntegrationSuite) profileThroughTheMiddleware(session string) *httptest.ResponseRecorder {
	s.T().Helper()
	h := s.newAuthHandler(false)
	api, _ := newWireTestAPI(s.T())
	huma.Register(api, huma.Operation{
		OperationID: "get-profile",
		Method:      http.MethodGet,
		Path:        "/auth/profile",
		Middlewares: huma.Middlewares{middleware.HumaJWTMiddleware(s.jwtService())},
	}, h.GetProfileHandler)
	return api.Get("/auth/profile", "Cookie: auth_token="+session)
}

func (s *AuthHandlerIntegrationSuite) requireProfileRefusedAsRevoked(session string) {
	s.T().Helper()
	w := s.profileThroughTheMiddleware(session)
	s.Equal(http.StatusUnauthorized, w.Code, "body: %s", w.Body.String())
	var body struct {
		ErrorCode string `json:"error_code"`
	}
	s.Require().NoError(json.Unmarshal(w.Body.Bytes(), &body), "body: %s", w.Body.String())
	s.Equal(autherrors.CodeTokenInvalid, body.ErrorCode)

	_, _, err := s.jwtService().ValidateSession(session)
	s.True(errors.Is(err, servicesshared.ErrSessionRevoked), "refused as revoked, not as closed: got %v", err)
}

func (s *AuthHandlerIntegrationSuite) requireProfileHonored(session string, userID uint) {
	s.T().Helper()
	w := s.profileThroughTheMiddleware(session)
	s.Equal(http.StatusOK, w.Code, "body: %s", w.Body.String())
	user, _, err := s.jwtService().ValidateSession(session)
	s.Require().NoError(err)
	s.Equal(userID, user.ID)
}

// deleteAccountSignedInAs closes user's account through the delete handler
// and returns a session minted for it before the deletion, which validation
// refuses while the account is closed.
func (s *AuthHandlerIntegrationSuite) deleteAccountSignedInAs(user *authm.User) string {
	s.T().Helper()
	before := s.sessionFor(user)
	s.Equal(http.StatusOK, s.profileThroughTheMiddleware(before).Code, "the session works before the deletion")

	input := &DeleteAccountRequest{}
	input.Body.Password = recoverySessionPassword
	resp, err := s.newAuthHandler(false).DeleteAccountHandler(s.ctxWithUser(s.reloadUser(user.ID)), input)
	s.Require().NoError(err)
	s.Require().True(resp.Body.Success, "delete: %s", resp.Body.Message)

	s.Equal(http.StatusUnauthorized, s.profileThroughTheMiddleware(before).Code, "the session is refused while the account is closed")
	return before
}

func (s *AuthHandlerIntegrationSuite) TestRecoverAccount_RefusesSessionsMintedBeforeTheDeletion() {
	user := s.createUserWithPassword("recover-revoked@test.com", recoverySessionPassword)
	before := s.deleteAccountSignedInAs(user)

	input := &RecoverAccountRequest{}
	input.Body.Email = "recover-revoked@test.com"
	input.Body.Password = recoverySessionPassword
	resp, err := s.newAuthHandler(false).RecoverAccountHandler(context.Background(), input)
	s.Require().NoError(err)
	s.Require().True(resp.Body.Success, "recover: %s", resp.Body.Message)
	s.Require().NotEmpty(resp.SetCookie.Value)

	s.requireProfileRefusedAsRevoked(before)
	s.requireProfileHonored(resp.SetCookie.Value, user.ID)
}

func (s *AuthHandlerIntegrationSuite) TestConfirmRecovery_RefusesSessionsMintedBeforeTheDeletion() {
	user := s.createUserWithPassword("confirm-revoked@test.com", recoverySessionPassword)
	before := s.deleteAccountSignedInAs(user)

	token, err := s.jwtService().CreateAccountRecoveryToken(user.ID, "confirm-revoked@test.com")
	s.Require().NoError(err)
	input := &ConfirmAccountRecoveryRequest{}
	input.Body.Token = token
	resp, err := s.newAuthHandler(false).ConfirmAccountRecoveryHandler(context.Background(), input)
	s.Require().NoError(err)
	s.Require().True(resp.Body.Success, "confirm: %s", resp.Body.Message)
	s.Require().NotEmpty(resp.SetCookie.Value)

	s.requireProfileRefusedAsRevoked(before)
	s.requireProfileHonored(resp.SetCookie.Value, user.ID)
}
