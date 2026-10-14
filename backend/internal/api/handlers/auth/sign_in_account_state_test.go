package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/golang-jwt/jwt/v5"
	"github.com/markbates/goth"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	authsvc "psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/contracts"
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
	{
		// Not a state the soft-delete writer leaves; here so deleted_at
		// refuses on its own.
		name: "deleted-at-only",
		apply: func(s *OAuthHandlerIntegrationSuite, userID uint) {
			s.Require().NoError(s.deps.DB.Model(&authm.User{}).
				Where("id = ?", userID).Update("deleted_at", time.Now()).Error)
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

func (p presetClaimsAppleService) ValidateIdentityToken(string) (*contracts.AppleIdentityTokenClaims, error) {
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
			created, err := svc.FindOrCreateAppleUser(svc.claims, "", "")
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

// The passkey refusal against real account rows: the account is loaded by the
// real user service and the session by the real JWT service, with only the
// WebAuthn ceremony itself standing in.
func (s *OAuthHandlerIntegrationSuite) TestPasskeyFinishLogin_AccountState() {
	inactive := autherrors.ErrAccountInactive()

	for _, state := range signInAccountStates {
		s.Run(state.name, func() {
			user := &authm.User{Email: strPtr("passkey-" + state.name + "@test.com"), IsActive: true, EmailVerified: true}
			s.Require().NoError(s.deps.DB.Create(user).Error)
			state.apply(s, user.ID)

			ceremony := &testhelpers.MockWebAuthnService{
				GetChallengeFn: func(string, string) (*webauthn.SessionData, uint, error) {
					return &webauthn.SessionData{}, user.ID, nil
				},
				FinishLoginFn: func(*authm.User, *webauthn.SessionData, *protocol.ParsedCredentialAssertionData) (*authm.WebAuthnCredential, error) {
					return &authm.WebAuthnCredential{}, nil
				},
			}
			h := NewPasskeyHandler(ceremony, s.jwtService(), s.deps.UserService, &testhelpers.MockEmailService{}, s.cfg)

			resp, err := h.FinishLoginHandler(context.Background(), finishLoginInput(s.T()))
			s.Require().NoError(err)

			if state.maySignIn {
				s.True(resp.Body.Success)
				s.NotEmpty(resp.SetCookie.Value)
				return
			}

			s.False(resp.Body.Success)
			s.Equal(autherrors.CodeAccountInactive, resp.Body.ErrorCode)
			s.Equal(inactive.UserMessage(), resp.Body.Message)
			s.Empty(resp.SetCookie.Value)
			s.Nil(resp.Body.User)
		})
	}
}

// finishLoginInput builds a FinishLoginRequest from the JSON a browser posts:
// an assertion that parses, carrying no valid signature. Verification itself
// is the mocked WebAuthn service's job in these tests.
func finishLoginInput(t *testing.T) *FinishLoginRequest {
	t.Helper()
	b64 := base64.RawURLEncoding.EncodeToString

	clientData, err := json.Marshal(map[string]string{
		"type":      "webauthn.get",
		"challenge": b64([]byte("account-state-challenge")),
		"origin":    "http://localhost:3000",
	})
	if err != nil {
		t.Fatal(err)
	}
	// rpIdHash (32 bytes), flags (user present), sign count (4 bytes).
	authData := append(make([]byte, 32), 0x01, 0, 0, 0, 1)

	body, err := json.Marshal(map[string]any{
		"challenge_id": "account-state-challenge-id",
		"response": map[string]any{
			"id":    b64([]byte("credential-id")),
			"rawId": b64([]byte("credential-id")),
			"type":  "public-key",
			"response": map[string]string{
				"authenticatorData": b64(authData),
				"clientDataJSON":    b64(clientData),
				"signature":         b64([]byte("signature")),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	input := &FinishLoginRequest{}
	if err := json.Unmarshal(body, &input.Body); err != nil {
		t.Fatal(err)
	}
	return input
}

func TestFinishLoginHandler_AccountState(t *testing.T) {
	deletedAt := time.Now()
	inactive := autherrors.ErrAccountInactive()

	states := []struct {
		name      string
		user      authm.User
		maySignIn bool
	}{
		{name: "active", user: authm.User{ID: 7, IsActive: true}, maySignIn: true},
		{name: "deactivated", user: authm.User{ID: 7, IsActive: false}},
		{name: "soft-deleted", user: authm.User{ID: 7, IsActive: false, DeletedAt: &deletedAt}},
		{name: "deleted-at-only", user: authm.User{ID: 7, IsActive: true, DeletedAt: &deletedAt}},
	}
	// The challenge's user id decides which ceremony runs: zero is a
	// discoverable login, anything else names the account up front.
	ceremonies := []struct {
		name          string
		challengeUser uint
	}{
		{name: "user-specific", challengeUser: 7},
		{name: "discoverable", challengeUser: 0},
	}

	for _, ceremony := range ceremonies {
		for _, state := range states {
			t.Run(ceremony.name+"/"+state.name, func(t *testing.T) {
				user := state.user
				challengeDeleted := false
				tokenMinted := false

				mockWA := &testhelpers.MockWebAuthnService{
					GetChallengeFn: func(string, string) (*webauthn.SessionData, uint, error) {
						return &webauthn.SessionData{}, ceremony.challengeUser, nil
					},
					FinishLoginFn: func(*authm.User, *webauthn.SessionData, *protocol.ParsedCredentialAssertionData) (*authm.WebAuthnCredential, error) {
						return &authm.WebAuthnCredential{}, nil
					},
					FinishDiscoverableLoginFn: func(*webauthn.SessionData, *protocol.ParsedCredentialAssertionData) (*authm.User, *authm.WebAuthnCredential, error) {
						return &user, &authm.WebAuthnCredential{}, nil
					},
					DeleteChallengeFn: func(string) error {
						challengeDeleted = true
						return nil
					},
				}
				mockUS := &testhelpers.MockUserService{
					GetUserByIDFn: func(uint) (*authm.User, error) { return &user, nil },
				}
				mockJWT := &testhelpers.MockJWTService{
					CreateTokenFn: func(*authm.User) (string, error) {
						tokenMinted = true
						return "session-token", nil
					},
				}
				h := testPasskeyHandlerWithMocks(mockWA, mockJWT, mockUS)

				resp, err := h.FinishLoginHandler(context.Background(), finishLoginInput(t))
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !challengeDeleted {
					t.Error("a verified assertion must consume its challenge")
				}

				if state.maySignIn {
					if !resp.Body.Success {
						t.Fatalf("expected success, got code=%s message=%q", resp.Body.ErrorCode, resp.Body.Message)
					}
					if resp.SetCookie.Value != "session-token" {
						t.Errorf("session cookie = %q, want the minted token", resp.SetCookie.Value)
					}
					return
				}

				if resp.Body.Success {
					t.Error("expected success=false")
				}
				if resp.Body.ErrorCode != autherrors.CodeAccountInactive {
					t.Errorf("error_code = %q, want %q", resp.Body.ErrorCode, autherrors.CodeAccountInactive)
				}
				if resp.Body.Message != inactive.UserMessage() {
					t.Errorf("message = %q, want %q", resp.Body.Message, inactive.UserMessage())
				}
				if tokenMinted {
					t.Error("a refused sign-in must not mint a session token")
				}
				if resp.SetCookie.Value != "" {
					t.Error("a refused sign-in must not set a session cookie")
				}
				if resp.Body.User != nil {
					t.Error("a refused sign-in must not return the account")
				}
			})
		}
	}
}
