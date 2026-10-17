package auth

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/golang-jwt/jwt/v5"
	"github.com/markbates/goth"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	authsvc "psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil/signupconsent"
	"psychic-homily-backend/internal/testutil/softauthn"
)

// An account that may not sign in (deactivated, or soft-deleted) is refused on
// the Google, Apple and passkey sign-in paths with ACCOUNT_INACTIVE, and no
// session is minted. An active account on the same path signs in.

// accountState puts an existing account into one of the states a sign-in path
// must tell apart.
type accountState struct {
	name      string
	apply     func(s *OAuthHandlerIntegrationSuite, userID uint)
	maySignIn bool
}

var signInAccountStates = []accountState{
	{
		name:      "active",
		apply:     func(*OAuthHandlerIntegrationSuite, uint) {},
		maySignIn: true,
	},
	{
		name: "deactivated",
		apply: func(s *OAuthHandlerIntegrationSuite, userID uint) {
			s.Require().NoError(s.deps.DB.Model(&authm.User{}).
				Where("id = ?", userID).Update("is_active", false).Error)
		},
	},
	{
		name: "soft-deleted",
		apply: func(s *OAuthHandlerIntegrationSuite, userID uint) {
			s.Require().NoError(s.deps.UserService.SoftDeleteAccount(userID, nil))
		},
	},
}

// createGoogleAccount creates an active account holding a Google identity.
func (s *OAuthHandlerIntegrationSuite) createGoogleAccount(address, subject string) *authm.User {
	s.T().Helper()
	user := &authm.User{Email: strPtr(address), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(user).Error)
	s.Require().NoError(s.deps.DB.Create(&authm.OAuthAccount{
		UserID: user.ID, Provider: "google", ProviderUserID: subject,
	}).Error)
	return user
}

func (s *OAuthHandlerIntegrationSuite) TestGoogleCallback_AccountState_WebRedirect() {
	for _, state := range signInAccountStates {
		s.Run(state.name, func() {
			address := "google-web-" + state.name + "@test.com"
			subject := "google-web-state-" + state.name
			user := s.createGoogleAccount(address, subject)
			state.apply(s, user.ID)

			handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
				Provider: "google", UserID: subject, Email: address,
			}})
			w, req := oauthCallbackRequest("google")
			handler.OAuthCallbackHTTPHandler(w, req)

			if state.maySignIn {
				s.assertSignedIn(w)
				return
			}

			s.assertAuthPageRefusal(w, autherrors.ErrAccountInactive())
		})
	}
}

func (s *OAuthHandlerIntegrationSuite) TestGoogleCallback_AccountState_CLIRedirect() {
	inactive := autherrors.ErrAccountInactive()

	for _, state := range signInAccountStates {
		s.Run(state.name, func() {
			address := "google-cli-" + state.name + "@test.com"
			subject := "google-cli-state-" + state.name
			user := s.createGoogleAccount(address, subject)
			state.apply(s, user.ID)

			callbackID := "cli-state-" + state.name
			storeCLICallback(callbackID, "http://localhost:8888/cli-state")

			handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
				Provider: "google", UserID: subject, Email: address,
			}})
			w, req := oauthCallbackRequest("google")
			req.AddCookie(&http.Cookie{Name: "cli_callback_id", Value: callbackID})
			handler.OAuthCallbackHTTPHandler(w, req)

			s.Equal(http.StatusTemporaryRedirect, w.Code)
			location, err := url.Parse(w.Header().Get("Location"))
			s.Require().NoError(err)
			s.Equal("http://localhost:8888/cli-state", location.Scheme+"://"+location.Host+location.Path)

			if state.maySignIn {
				s.NotEmpty(location.Query().Get("token"))
				return
			}

			// The loopback redirect carries the message alone, as every CLI
			// error redirect does, and never a token.
			s.Equal(url.Values{"error": {inactive.UserMessage()}}, location.Query())
			s.assertNoSessionCookie(w)
		})
	}
}

// presetClaimsAppleService is the real Apple service with token verification
// replaced, so the handler runs against real account rows without Apple's
// JWKS.
type presetClaimsAppleService struct {
	*authsvc.AppleAuthService
	claims *contracts.AppleIdentityTokenClaims
}

func (p presetClaimsAppleService) ValidateIdentityToken(context.Context, string) (*contracts.AppleIdentityTokenClaims, error) {
	return p.claims, nil
}

func (s *OAuthHandlerIntegrationSuite) TestAppleCallback_AccountState() {
	inactive := autherrors.ErrAccountInactive()

	for _, state := range signInAccountStates {
		s.Run(state.name, func() {
			svc := presetClaimsAppleService{
				AppleAuthService: authsvc.NewAppleAuthService(s.deps.DB, s.cfg, s.jwtService()),
				claims: &contracts.AppleIdentityTokenClaims{
					Email:            "apple-" + state.name + "@test.com",
					EmailVerified:    true,
					RegisteredClaims: jwt.RegisteredClaims{Subject: "apple-state-" + state.name},
				},
			}
			created, err := svc.FindOrCreateAppleUser(svc.claims, "", "", signupconsent.CurrentOAuthSignupConsent())
			s.Require().NoError(err)
			state.apply(s, created.ID)

			h := NewAppleAuthHandler(svc, &testhelpers.MockDiscordService{}, s.cfg)
			input := &AppleCallbackRequest{}
			input.Body.IdentityToken = "preset"

			resp, err := h.AppleCallbackHandler(context.Background(), input)
			s.Require().NoError(err)

			if state.maySignIn {
				s.True(resp.Body.Success)
				s.NotEmpty(resp.Body.Token)
				s.NotEmpty(resp.SetCookie.Value)
				return
			}

			s.False(resp.Body.Success)
			s.Equal(autherrors.CodeAccountInactive, resp.Body.ErrorCode)
			s.Equal(inactive.UserMessage(), resp.Body.Message)
			s.Empty(resp.Body.Token)
			s.Nil(resp.Body.User)
			s.Empty(resp.SetCookie.Value)
		})
	}
}

// The passkey refusal against real account rows and a real signature: a
// software authenticator signs the assertion, the real WebAuthn service
// verifies it and decides on the account, and the real JWT service mints any
// session, on both ceremonies.
func (s *OAuthHandlerIntegrationSuite) TestPasskeyFinishLogin_AccountState() {
	inactive := autherrors.ErrAccountInactive()
	webauthnService := newTestWebAuthnService(s.T(), s.deps.DB)
	h := NewPasskeyHandler(webauthnService, s.jwtService(), s.deps.UserService, &testhelpers.MockEmailService{}, s.cfg)

	// Each ceremony begins a login and stores its challenge as login/begin
	// does: under the account's id for an email-specific login, under zero for
	// a discoverable one.
	type begun struct {
		options       *protocol.CredentialAssertion
		session       *webauthn.SessionData
		challengeUser uint
	}
	ceremonies := []struct {
		name  string
		begin func(userID uint) begun
	}{
		{name: "email", begin: func(userID uint) begun {
			options, session, err := webauthnService.BeginLogin(&authm.User{ID: userID})
			s.Require().NoError(err)
			return begun{options, session, userID}
		}},
		{name: "discoverable", begin: func(uint) begun {
			options, session, err := webauthnService.BeginDiscoverableLogin()
			s.Require().NoError(err)
			return begun{options, session, 0}
		}},
	}

	for _, ceremony := range ceremonies {
		for _, state := range signInAccountStates {
			s.Run(ceremony.name+"/"+state.name, func() {
				user := &authm.User{Email: strPtr("passkey-" + ceremony.name + "-" + state.name + "@test.com"), IsActive: true, EmailVerified: true}
				s.Require().NoError(s.deps.DB.Create(user).Error)
				device := s.registerSoftPasskey(webauthnService, user)
				state.apply(s, user.ID)

				login := ceremony.begin(user.ID)
				challengeID, err := webauthnService.StoreChallenge(login.challengeUser, login.session, "authentication")
				s.Require().NoError(err)
				assertion, err := device.Assert(login.options)
				s.Require().NoError(err)
				input := &FinishLoginRequest{}
				input.Body.ChallengeID = challengeID
				input.Body.Response = assertion

				resp, err := h.FinishLoginHandler(context.Background(), input)
				s.Require().NoError(err)

				var stored authm.WebAuthnCredential
				s.Require().NoError(s.deps.DB.Where("user_id = ?", user.ID).First(&stored).Error)

				if state.maySignIn {
					s.True(resp.Body.Success, resp.Body.Message)
					s.Require().NotNil(resp.Body.User)
					s.Equal(user.ID, resp.Body.User.ID)
					s.Empty(resp.Body.User.PasskeyCredentials)
					s.NotEmpty(resp.SetCookie.Value)
					s.NotNil(stored.LastUsedAt)
					return
				}

				s.False(resp.Body.Success)
				s.Equal(autherrors.CodeAccountInactive, resp.Body.ErrorCode)
				s.Equal(inactive.UserMessage(), resp.Body.Message)
				s.Empty(resp.SetCookie.Value)
				s.Nil(resp.Body.User)
				s.Nil(stored.LastUsedAt, "a refused sign-in records no use of the credential")
			})
		}
	}
}

// registerSoftPasskey registers a fresh software authenticator for user
// through the WebAuthn service and returns it.
func (s *OAuthHandlerIntegrationSuite) registerSoftPasskey(webauthnService *authsvc.WebAuthnService, user *authm.User) *softauthn.Authenticator {
	s.T().Helper()
	device := softauthn.New(wireOrigin)
	options, session, err := webauthnService.BeginRegistration(user)
	s.Require().NoError(err)
	body, err := device.Register(options)
	s.Require().NoError(err)
	parsed, err := protocol.ParseCredentialCreationResponseBytes(body)
	s.Require().NoError(err)
	_, err = webauthnService.FinishRegistration(user, session, parsed, "Key")
	s.Require().NoError(err)
	return device
}
