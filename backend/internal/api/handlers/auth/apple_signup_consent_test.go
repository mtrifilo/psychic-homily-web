package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	authsvc "psychic-homily-backend/internal/services/auth"
)

// The Apple callback end to end: a validly signed identity token, verified by
// the real Apple service against a stubbed key endpoint, resolved against real
// account rows. An unknown identity becomes an account only with consent in
// the body; a known one signs in without it. The full set of refusable consent
// shapes is pinned against the shared validator; these pin the wiring.

const appleConsentKid = "consent-kid"

// appleConsentCallback is a handler over the real Apple service, and a
// function signing identity tokens that service accepts.
func (s *OAuthHandlerIntegrationSuite) appleConsentCallback() (*AppleAuthHandler, func(sub, email string) string) {
	s.T().Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	s.Require().NoError(err)
	stubAppleKeyEndpoint(s.T(), appleKeySet(key, appleConsentKid))

	cfg := *s.cfg
	cfg.Apple.BundleID = "com.psychichomily.ios"
	svc := authsvc.NewAppleAuthService(s.deps.DB, &cfg, s.jwtService())
	sign := func(sub, email string) string {
		return signedAppleTokenFor(s.T(), key, cfg.Apple.BundleID, appleConsentKid, sub, email, time.Now().Add(time.Hour))
	}
	return NewAppleAuthHandler(svc, &testhelpers.MockDiscordService{}, &cfg), sign
}

// withCurrentConsent fills input's consent fields as a client that showed the
// current terms and the age confirmation sends them.
func withCurrentConsent(input *AppleCallbackRequest) {
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = legal.CurrentTermsVersion
	input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge
}

func (s *OAuthHandlerIntegrationSuite) TestAppleCallback_NewIdentityWithoutConsent_Refused() {
	h, sign := s.appleConsentCallback()

	for name, tc := range map[string]struct {
		fill func(*AppleCallbackRequest)
		want *autherrors.AuthError
	}{
		"no consent fields": {
			func(*AppleCallbackRequest) {},
			autherrors.ErrTermsAcceptanceRequired(""),
		},
		"terms not accepted": {
			func(in *AppleCallbackRequest) { withCurrentConsent(in); in.Body.TermsAccepted = false },
			autherrors.ErrTermsAcceptanceRequired(""),
		},
		"stale terms version": {
			func(in *AppleCallbackRequest) { withCurrentConsent(in); in.Body.TermsVersion = "2025-01-01" },
			autherrors.ErrTermsAcceptanceRequired(""),
		},
		"age not confirmed": {
			func(in *AppleCallbackRequest) { withCurrentConsent(in); in.Body.AgeConfirmed = false },
			autherrors.ErrAgeConfirmationRequired(""),
		},
		"attested age below the minimum": {
			func(in *AppleCallbackRequest) { withCurrentConsent(in); in.Body.MinAgeAttested = MinSignupAge - 1 },
			autherrors.ErrAgeConfirmationRequired(""),
		},
	} {
		s.Run(name, func() {
			input := &AppleCallbackRequest{}
			input.Body.IdentityToken = sign("apple-sub-new", "apple-new@test.com")
			tc.fill(input)

			resp, err := h.AppleCallbackHandler(context.Background(), input)
			s.Require().NoError(err)
			s.False(resp.Body.Success)
			s.Equal(tc.want.Code, resp.Body.ErrorCode)
			s.Equal(tc.want.UserMessage(), resp.Body.Message)
			s.Empty(resp.Body.Token)
			s.Nil(resp.Body.User)
			s.Empty(resp.SetCookie.Value)

			var users, identities int64
			s.Require().NoError(s.deps.DB.Model(&authm.User{}).Count(&users).Error)
			s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).Count(&identities).Error)
			s.Zero(users, "a refused creation writes no user")
			s.Zero(identities, "a refused creation writes no identity")
		})
	}
}

func (s *OAuthHandlerIntegrationSuite) TestAppleCallback_NewIdentityWithConsent_CreatesAndRecords() {
	h, sign := s.appleConsentCallback()
	input := &AppleCallbackRequest{}
	input.Body.IdentityToken = sign("apple-sub-consented", "apple-consented@test.com")
	withCurrentConsent(input)

	resp, err := h.AppleCallbackHandler(context.Background(), input)
	s.Require().NoError(err)
	s.Require().True(resp.Body.Success, "code=%s message=%q", resp.Body.ErrorCode, resp.Body.Message)
	s.NotEmpty(resp.Body.Token)
	s.NotEmpty(resp.SetCookie.Value)
	s.Require().NotNil(resp.Body.User)

	var stored authm.User
	s.Require().NoError(s.deps.DB.First(&stored, resp.Body.User.ID).Error)
	s.Require().NotNil(stored.TermsVersion)
	s.Require().NotNil(stored.PrivacyVersion)
	s.Require().NotNil(stored.TermsAcceptedAt)
	s.Require().NotNil(stored.AgeConfirmedAt)
	s.Require().NotNil(stored.MinAgeAttested)
	s.Equal(legal.CurrentTermsVersion, *stored.TermsVersion)
	s.Equal(legal.CurrentPrivacyVersion, *stored.PrivacyVersion)
	s.WithinDuration(time.Now(), *stored.TermsAcceptedAt, time.Minute)
	s.True(stored.AgeConfirmedAt.Equal(*stored.TermsAcceptedAt))
	s.Equal(MinSignupAge, *stored.MinAgeAttested)
}

func (s *OAuthHandlerIntegrationSuite) TestAppleCallback_ReturningIdentityWithoutConsent_SignsIn() {
	h, sign := s.appleConsentCallback()
	first := &AppleCallbackRequest{}
	first.Body.IdentityToken = sign("apple-sub-returning", "apple-returning@test.com")
	withCurrentConsent(first)
	created, err := h.AppleCallbackHandler(context.Background(), first)
	s.Require().NoError(err)
	s.Require().True(created.Body.Success)

	again := &AppleCallbackRequest{}
	again.Body.IdentityToken = sign("apple-sub-returning", "apple-returning@test.com")

	resp, err := h.AppleCallbackHandler(context.Background(), again)
	s.Require().NoError(err)
	s.Require().True(resp.Body.Success, "code=%s message=%q", resp.Body.ErrorCode, resp.Body.Message)
	s.NotEmpty(resp.Body.Token)
	s.Equal(created.Body.User.ID, resp.Body.User.ID)
}

// The JSON contract through Huma's decoding and schema validation: a body with
// the identity token alone still signs in an account that holds no legal
// record (as every Apple account created before consent was required does)
// and is refused for a new identity, and the snake_case consent fields create
// the account.
func (s *OAuthHandlerIntegrationSuite) TestAppleCallback_Wire() {
	h, sign := s.appleConsentCallback()
	api, _ := newWireTestAPI(s.T())
	huma.Post(api, "/auth/apple/callback", h.AppleCallbackHandler)

	legacy := &authm.User{Email: strPtr("apple-legacy@test.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.deps.DB.Create(legacy).Error)
	s.Require().NoError(s.deps.DB.Create(&authm.OAuthAccount{UserID: legacy.ID, Provider: "apple", ProviderUserID: "apple-sub-legacy"}).Error)

	s.Run("token alone signs in an account without a legal record", func() {
		resp := api.Post("/auth/apple/callback", map[string]any{"identity_token": sign("apple-sub-legacy", "apple-legacy@test.com")})
		requireWireSuccess(s.T(), resp)
		user, _ := decodeWireBody(s.T(), resp.Body.Bytes())["user"].(map[string]any)
		s.InDelta(float64(legacy.ID), user["id"], 0)

		var stored authm.User
		s.Require().NoError(s.deps.DB.First(&stored, legacy.ID).Error)
		s.Nil(stored.TermsVersion)
		s.Nil(stored.AgeConfirmedAt)
		s.Nil(stored.MinAgeAttested)
	})

	s.Run("token alone is refused for a new identity", func() {
		resp := api.Post("/auth/apple/callback", map[string]any{"identity_token": sign("apple-sub-wire-new", "apple-wire-new@test.com")})
		s.Equal(http.StatusOK, resp.Code, resp.Body.String())
		body := decodeWireBody(s.T(), resp.Body.Bytes())
		s.Equal(false, body["success"])
		s.Equal(autherrors.CodeTermsAcceptanceRequired, body["error_code"])

		var identities int64
		s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).Where("provider_user_id = ?", "apple-sub-wire-new").Count(&identities).Error)
		s.Zero(identities)
	})

	s.Run("snake_case consent creates the account", func() {
		resp := api.Post("/auth/apple/callback", map[string]any{
			"identity_token":   sign("apple-sub-wire-consent", "apple-wire-consent@test.com"),
			"terms_accepted":   true,
			"terms_version":    legal.CurrentTermsVersion,
			"privacy_version":  legal.CurrentPrivacyVersion,
			"age_confirmed":    true,
			"min_age_attested": MinSignupAge,
		})
		requireWireSuccess(s.T(), resp)

		var stored authm.User
		s.Require().NoError(s.deps.DB.Where("email = ?", "apple-wire-consent@test.com").First(&stored).Error)
		s.Require().NotNil(stored.TermsVersion)
		s.Require().NotNil(stored.PrivacyVersion)
		s.Require().NotNil(stored.MinAgeAttested)
		s.Equal(legal.CurrentTermsVersion, *stored.TermsVersion)
		s.Equal(legal.CurrentPrivacyVersion, *stored.PrivacyVersion)
		s.Equal(MinSignupAge, *stored.MinAgeAttested)
	})
}
