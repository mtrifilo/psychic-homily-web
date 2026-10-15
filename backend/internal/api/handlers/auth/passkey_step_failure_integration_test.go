package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/go-webauthn/webauthn/protocol"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testlog"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/testutil/softauthn"
)

// A begin step, or the credential list, that cannot read or write what it
// needs answers 503 and hands out no challenge.
func (s *AuthHandlerIntegrationSuite) TestPasskeyBeginAndListFailuresAnswerServiceUnavailable() {
	jwt := &testhelpers.MockJWTService{}
	challengeStoreFails := func() *PasskeyHandler {
		testutil.FailInserts(s.T(), s.deps.DB, "webauthn_challenges")
		return s.newRealPasskeyHandler()
	}
	webauthnUnavailable := func() *PasskeyHandler {
		return s.newPasskeyHandlerOver(newTestWebAuthnService(s.T(), testutil.UnavailableDB(s.T())))
	}
	usersUnavailable := func() *PasskeyHandler {
		return NewPasskeyHandler(newTestWebAuthnService(s.T(), s.deps.DB), jwt,
			usersvc.NewUserService(testutil.UnavailableDB(s.T())), &testhelpers.MockEmailService{}, testConfig())
	}
	registerBegin := func(api humatest.TestAPI) *httptest.ResponseRecorder {
		return api.Post("/auth/passkey/register/begin", map[string]any{"display_name": "Laptop"})
	}
	signupBegin := func(api humatest.TestAPI) *httptest.ResponseRecorder {
		return api.Post("/auth/passkey/signup/begin", signupBeginBody("begin-fails@example.com"))
	}

	cases := []struct {
		name    string
		handler func() *PasskeyHandler
		send    func(humatest.TestAPI) *httptest.ResponseRecorder
	}{
		{"register begin cannot read existing credentials", webauthnUnavailable, registerBegin},
		{"register begin cannot store its challenge", challengeStoreFails, registerBegin},
		{"credential list cannot be read", webauthnUnavailable, func(api humatest.TestAPI) *httptest.ResponseRecorder {
			return api.Get("/auth/passkey/credentials")
		}},
		{"credential delete cannot run", webauthnUnavailable, func(api humatest.TestAPI) *httptest.ResponseRecorder {
			return api.Delete("/auth/passkey/credentials/5")
		}},
		{"signup begin cannot check the address", usersUnavailable, signupBegin},
		{"signup begin cannot reserve the account id", webauthnUnavailable, signupBegin},
		{"signup begin cannot store its challenge", challengeStoreFails, signupBegin},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			resp := tc.send(newPasskeyWireAPI(s.T(), tc.handler()))

			s.requireServiceUnavailable(resp)
			s.NotContains(decodeWireBody(s.T(), resp.Body.Bytes()), "challenge_id")
		})
	}
}

// Deleting a credential the account does not hold is refused, not failed.
func (s *AuthHandlerIntegrationSuite) TestPasskeyDeleteRefusesCredentialNotHeld() {
	api := newPasskeyWireAPI(s.T(), s.newRealPasskeyHandler())

	resp := api.Delete("/auth/passkey/credentials/999999")

	requireWireOutcome(s.T(), resp, wireOutcome{http.StatusOK, autherrors.CodeValidationFailed, "Failed to delete credential"}, "not held")
}

// registerPasskey runs register begin and finish for user, answering the
// options with answer.
func (s *AuthHandlerIntegrationSuite) registerPasskey(h *PasskeyHandler, user *authm.User, answer func(*protocol.CredentialCreation) ([]byte, error)) *FinishRegisterResponse {
	s.T().Helper()
	ctx := testhelpers.CtxWithSessionAuthTime(user, time.Now())
	begin, err := h.BeginRegisterHandler(ctx, &BeginRegisterRequest{})
	s.Require().NoError(err)
	s.Require().True(begin.Body.Success, begin.Body.Message)
	registration, err := answer(begin.Body.Options)
	s.Require().NoError(err)
	input := &FinishRegisterRequest{}
	input.Body.ChallengeID = begin.Body.ChallengeID
	input.Body.Response = registration
	resp, err := h.FinishRegisterHandler(ctx, input)
	s.Require().NoError(err)
	return resp
}

// Registering a credential id an account already holds is refused as an
// invalid credential response and stores nothing, whichever account holds it.
func (s *AuthHandlerIntegrationSuite) TestPasskeyRegisterFinishRefusesHeldCredential() {
	h := s.newRealPasskeyHandler()
	newUser := func(email string) *authm.User {
		user := &authm.User{Email: &email, IsActive: true}
		s.Require().NoError(s.deps.DB.Create(user).Error)
		return user
	}
	holder := newUser("held-credential@example.com")
	other := newUser("held-elsewhere@example.com")
	device := softauthn.New(wireOrigin)
	first := s.registerPasskey(h, holder, device.Register)
	s.Require().True(first.Body.Success, first.Body.Message)

	for name, user := range map[string]*authm.User{
		"by the account that holds it": holder,
		"by another account":           other,
	} {
		s.Run(name, func() {
			before := s.credentialCount(user.ID)

			refused := s.registerPasskey(h, user, device.RegisterAgain)

			s.False(refused.Body.Success)
			s.Equal(autherrors.CodeValidationFailed, refused.Body.ErrorCode)
			s.Equal("Invalid credential response", refused.Body.Message)
			s.Nil(refused.Body.Credential)
			s.Equal(before, s.credentialCount(user.ID))
		})
	}
	s.Equal(int64(1), s.credentialCount(holder.ID))
}

// A magic-link request for an address holding a NUL byte is answered and
// logged as one for an address no account holds, not as a failed lookup.
func (s *AuthHandlerIntegrationSuite) TestSendMagicLink_NULAddressIsUnknown() {
	h := s.newAuthHandler(true)
	input := &SendMagicLinkRequest{}
	input.Body.Email = "no\x00body@test.com"

	var resp *SendMagicLinkResponse
	var err error
	logs := testlog.Capture(s.T(), func() {
		resp, err = h.SendMagicLinkHandler(testlog.Context(s.T(), context.Background()), input)
	})

	s.Require().NoError(err)
	s.True(resp.Body.Success)
	s.Empty(resp.Body.ErrorCode)
	s.Contains(logs, "magic_link_user_not_found")
	s.NotContains(logs, "magic_link_user_lookup_failed")
}
