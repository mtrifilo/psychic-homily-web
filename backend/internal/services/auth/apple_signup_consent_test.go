package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

// An Apple account is created only with consent to the current Terms of
// Service and Privacy Policy and the minimum age attested, and records that
// consent. A returning Apple identity signs in without any.

// signedAppleClaims is the claims of an identity token for subject and email,
// signed by a key the service fetches from a stub key server and verified by
// the real ValidateIdentityToken, so each test starts where the callback does.
func (s *AppleAuthIntegrationTestSuite) signedAppleClaims(svc *AppleAuthService, subject, email string) *contracts.AppleIdentityTokenClaims {
	s.T().Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	s.Require().NoError(err)
	keys := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jwks(key, "consent-kid"))
	}))
	s.T().Cleanup(keys.Close)
	svc.fetchAppleKeysFromURL = keys.URL

	token := createAppleIdentityToken(s.T(), key, "consent-kid", s.cfg.Apple.BundleID, subject, email, time.Now().Add(time.Hour))
	claims, err := svc.ValidateIdentityToken(s.T().Context(), token)
	s.Require().NoError(err)
	return claims
}

func (s *AppleAuthIntegrationTestSuite) countRows(model any) int64 {
	s.T().Helper()
	var n int64
	s.Require().NoError(s.db.Model(model).Count(&n).Error)
	return n
}

func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_CreationWithoutConsent_RefusedAndWritesNothing() {
	for name, tc := range map[string]struct {
		consent  func() *contracts.OAuthSignupConsent
		wantCode string
	}{
		"no consent": {
			func() *contracts.OAuthSignupConsent { return nil },
			apperrors.CodeTermsAcceptanceRequired,
		},
		"empty consent, as a client that sends no consent fields": {
			func() *contracts.OAuthSignupConsent {
				return &contracts.OAuthSignupConsent{AcceptedAt: time.Now().UTC()}
			},
			apperrors.CodeTermsAcceptanceRequired,
		},
		"terms not accepted": {
			withConsent(func(c *contracts.OAuthSignupConsent) { c.TermsAccepted = false }),
			apperrors.CodeTermsAcceptanceRequired,
		},
		"stale terms version": {
			withConsent(func(c *contracts.OAuthSignupConsent) { c.TermsVersion = "2025-01-01" }),
			apperrors.CodeTermsAcceptanceRequired,
		},
		"stale privacy version": {
			withConsent(func(c *contracts.OAuthSignupConsent) { c.PrivacyVersion = "2025-01-01" }),
			apperrors.CodeTermsAcceptanceRequired,
		},
		"age not confirmed": {
			withConsent(func(c *contracts.OAuthSignupConsent) { c.AgeConfirmed = false }),
			apperrors.CodeAgeConfirmationRequired,
		},
		"attested age below the minimum": {
			withConsent(func(c *contracts.OAuthSignupConsent) { c.MinAgeAttested = 15 }),
			apperrors.CodeAgeConfirmationRequired,
		},
	} {
		s.Run(name, func() {
			svc := s.newService()
			claims := s.signedAppleClaims(svc, "apple-sub-no-consent", "no-consent@example.com")

			user, err := svc.FindOrCreateAppleUser(claims, "No", "Consent", tc.consent())

			s.Require().Error(err)
			s.Nil(user)
			var authErr *apperrors.AuthError
			s.Require().ErrorAs(err, &authErr)
			s.Equal(tc.wantCode, authErr.Code)
			s.Zero(s.countRows(&authm.User{}), "a refused creation writes no user")
			s.Zero(s.countRows(&authm.OAuthAccount{}), "a refused creation writes no identity")
			s.Zero(s.countRows(&authm.UserPreferences{}), "a refused creation writes no preferences")
		})
	}
}

func withConsent(mutate func(*contracts.OAuthSignupConsent)) func() *contracts.OAuthSignupConsent {
	return func() *contracts.OAuthSignupConsent {
		consent := signupconsent.CurrentOAuthSignupConsent()
		mutate(consent)
		return consent
	}
}

func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_CreationWithConsent_RecordsIt() {
	svc := s.newService()
	claims := s.signedAppleClaims(svc, "apple-sub-consent", "consent@example.com")
	consent := signupconsent.CurrentOAuthSignupConsent()

	user, err := svc.FindOrCreateAppleUser(claims, "With", "Consent", consent)
	s.Require().NoError(err)

	var stored authm.User
	s.Require().NoError(s.db.First(&stored, user.ID).Error)
	s.Require().NotNil(stored.TermsVersion)
	s.Require().NotNil(stored.PrivacyVersion)
	s.Require().NotNil(stored.TermsAcceptedAt)
	s.Require().NotNil(stored.AgeConfirmedAt)
	s.Require().NotNil(stored.MinAgeAttested)
	s.Equal(legal.CurrentTermsVersion, *stored.TermsVersion)
	s.Equal(legal.CurrentPrivacyVersion, *stored.PrivacyVersion)
	s.WithinDuration(consent.AcceptedAt, *stored.TermsAcceptedAt, time.Millisecond)
	s.WithinDuration(consent.AcceptedAt, *stored.AgeConfirmedAt, time.Millisecond)
	s.Equal(consent.MinAgeAttested, *stored.MinAgeAttested)
	s.Equal(int64(1), s.countRows(&authm.OAuthAccount{}))
}

// The consent is not read for a returning identity, so neither its absence nor
// a refusable one stops the sign-in, and the record made at creation stands.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_ReturningIdentity_SignsInWithoutConsent() {
	svc := s.newService()
	claims := s.signedAppleClaims(svc, "apple-sub-returning-consent", "returning-consent@example.com")
	created, err := svc.FindOrCreateAppleUser(claims, "", "", signupconsent.CurrentOAuthSignupConsent())
	s.Require().NoError(err)
	var before authm.User
	s.Require().NoError(s.db.First(&before, created.ID).Error)

	for name, consent := range map[string]*contracts.OAuthSignupConsent{
		"no consent":      nil,
		"empty consent":   {},
		"refusable stale": withConsent(func(c *contracts.OAuthSignupConsent) { c.TermsVersion = "2025-01-01" })(),
	} {
		s.Run(name, func() {
			user, err := svc.FindOrCreateAppleUser(claims, "", "", consent)
			s.Require().NoError(err)
			s.Equal(created.ID, user.ID)

			var after authm.User
			s.Require().NoError(s.db.First(&after, created.ID).Error)
			s.Equal(*before.TermsVersion, *after.TermsVersion)
			s.Equal(*before.PrivacyVersion, *after.PrivacyVersion)
			s.True(before.TermsAcceptedAt.Equal(*after.TermsAcceptedAt))
			s.True(before.AgeConfirmedAt.Equal(*after.AgeConfirmedAt))
			s.Equal(*before.MinAgeAttested, *after.MinAgeAttested)
		})
	}
	s.Equal(int64(1), s.countRows(&authm.User{}))
}

// An address that already belongs to an account refuses the sign-in before the
// consent is consulted, as the Google path does, so a client with no consent
// step is told the address is taken rather than asked for consent it cannot
// use.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_HeldAddress_RefusedBeforeConsent() {
	existing := &authm.User{Email: stringPtr("held-consent@example.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(existing).Error)

	svc := s.newService()
	claims := s.signedAppleClaims(svc, "apple-sub-held-consent", "held-consent@example.com")

	user, err := svc.FindOrCreateAppleUser(claims, "", "", nil)

	s.Require().Error(err)
	s.Nil(user)
	var authErr *apperrors.AuthError
	s.Require().ErrorAs(err, &authErr)
	s.Equal(apperrors.CodeOAuthLinkRefused, authErr.Code)
	s.assertNoAppleAccountFor(existing.ID)
}
