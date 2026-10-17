package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	authsvc "psychic-homily-backend/internal/services/auth"
)

// The Apple callback end to end: a validly signed identity token, verified by
// the real Apple service against a stubbed key endpoint, resolved against real
// account rows. An unknown identity becomes an account only with consent in
// the body; a known one signs in without it.

const appleConsentKid = "consent-kid"

// appleConsentCallback is a handler over the real Apple service and the
// token-signing key its stubbed key endpoint publishes.
func (s *OAuthHandlerIntegrationSuite) appleConsentCallback() (*AppleAuthHandler, *rsa.PrivateKey, string) {
	s.T().Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	s.Require().NoError(err)
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	stubAppleKeyEndpoint(s.T(), []byte(`{"keys":[{"kty":"RSA","kid":"`+appleConsentKid+`","use":"sig","alg":"RS256","n":"`+n+`","e":"`+e+`"}]}`))

	cfg := *s.cfg
	cfg.Apple.BundleID = "com.psychichomily.ios"
	svc := authsvc.NewAppleAuthService(s.deps.DB, &cfg, s.jwtService())
	return NewAppleAuthHandler(svc, &testhelpers.MockDiscordService{}, &cfg), key, cfg.Apple.BundleID
}

// signedAppleIdentityToken is an identity token for sub and email, signed by
// key under appleConsentKid and valid for an hour.
func signedAppleIdentityToken(t *testing.T, key *rsa.PrivateKey, aud, sub, email string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":            "https://appleid.apple.com",
		"aud":            aud,
		"sub":            sub,
		"email":          email,
		"email_verified": "true",
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Unix(),
	})
	token.Header["kid"] = appleConsentKid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
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

func (s *OAuthHandlerIntegrationSuite) assertAppleRefusal(resp *AppleCallbackResponse, want *autherrors.AuthError) {
	s.T().Helper()
	s.False(resp.Body.Success)
	s.Equal(want.Code, resp.Body.ErrorCode)
	s.Equal(want.UserMessage(), resp.Body.Message)
	s.Empty(resp.Body.Token)
	s.Nil(resp.Body.User)
	s.Empty(resp.SetCookie.Value)

	var users, identities int64
	s.Require().NoError(s.deps.DB.Model(&authm.User{}).Count(&users).Error)
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthAccount{}).Count(&identities).Error)
	s.Zero(users, "a refused creation writes no user")
	s.Zero(identities, "a refused creation writes no identity")
}

func (s *OAuthHandlerIntegrationSuite) TestAppleCallback_NewIdentityWithoutConsent_Refused() {
	h, key, aud := s.appleConsentCallback()

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
			input.Body.IdentityToken = signedAppleIdentityToken(s.T(), key, aud, "apple-sub-new", "apple-new@test.com")
			tc.fill(input)

			resp, err := h.AppleCallbackHandler(context.Background(), input)
			s.Require().NoError(err)
			s.assertAppleRefusal(resp, tc.want)
		})
	}
}

func (s *OAuthHandlerIntegrationSuite) TestAppleCallback_NewIdentityWithConsent_CreatesAndRecords() {
	h, key, aud := s.appleConsentCallback()
	input := &AppleCallbackRequest{}
	input.Body.IdentityToken = signedAppleIdentityToken(s.T(), key, aud, "apple-sub-consented", "apple-consented@test.com")
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
	s.Require().NotNil(stored.MinAgeAttested)
	s.Equal(legal.CurrentTermsVersion, *stored.TermsVersion)
	s.Equal(legal.CurrentPrivacyVersion, *stored.PrivacyVersion)
	s.NotNil(stored.TermsAcceptedAt)
	s.NotNil(stored.AgeConfirmedAt)
	s.Equal(MinSignupAge, *stored.MinAgeAttested)
}

func (s *OAuthHandlerIntegrationSuite) TestAppleCallback_ReturningIdentityWithoutConsent_SignsIn() {
	h, key, aud := s.appleConsentCallback()
	first := &AppleCallbackRequest{}
	first.Body.IdentityToken = signedAppleIdentityToken(s.T(), key, aud, "apple-sub-returning", "apple-returning@test.com")
	withCurrentConsent(first)
	created, err := h.AppleCallbackHandler(context.Background(), first)
	s.Require().NoError(err)
	s.Require().True(created.Body.Success)

	again := &AppleCallbackRequest{}
	again.Body.IdentityToken = signedAppleIdentityToken(s.T(), key, aud, "apple-sub-returning", "apple-returning@test.com")

	resp, err := h.AppleCallbackHandler(context.Background(), again)
	s.Require().NoError(err)
	s.Require().True(resp.Body.Success, "code=%s message=%q", resp.Body.ErrorCode, resp.Body.Message)
	s.NotEmpty(resp.Body.Token)
	s.Equal(created.Body.User.ID, resp.Body.User.ID)
}
