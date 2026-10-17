package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/golang-jwt/jwt/v5"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

// An Apple account is created only with consent to the current Terms of
// Service and Privacy Policy and the minimum age attested, and records that
// consent. A returning Apple identity signs in without any. The full set of
// refusable consent shapes is pinned against the shared validator; these pin
// where this service consults it.

func (s *AppleAuthIntegrationTestSuite) countRows(model any) int64 {
	s.T().Helper()
	var n int64
	s.Require().NoError(s.db.Model(model).Count(&n).Error)
	return n
}

func appleClaims(subject, email string) *contracts.AppleIdentityTokenClaims {
	return &contracts.AppleIdentityTokenClaims{
		Email:            email,
		EmailVerified:    true,
		RegisteredClaims: jwt.RegisteredClaims{Subject: subject},
	}
}

// The claims come from a validly signed token verified by the real
// ValidateIdentityToken, so the refusal starts where the callback does.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_CreationWithoutConsent_RefusedAndWritesNothing() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	s.Require().NoError(err)
	keys := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jwks(key, "consent-kid"))
	}))
	defer keys.Close()
	svc := s.newService()
	svc.fetchAppleKeysFromURL = keys.URL
	token := createAppleIdentityToken(s.T(), key, "consent-kid", s.cfg.Apple.BundleID, "apple-sub-no-consent", "no-consent@example.com", time.Now().Add(time.Hour))
	claims, err := svc.ValidateIdentityToken(s.T().Context(), token)
	s.Require().NoError(err)

	for name, tc := range map[string]struct {
		consent  func() *contracts.OAuthSignupConsent
		wantCode string
	}{
		"no consent": {
			func() *contracts.OAuthSignupConsent { return nil },
			apperrors.CodeTermsAcceptanceRequired,
		},
		"empty consent, as a client that sends no consent fields": {
			func() *contracts.OAuthSignupConsent { return &contracts.OAuthSignupConsent{} },
			apperrors.CodeTermsAcceptanceRequired,
		},
		"stale privacy version": {
			signupconsent.CurrentOAuthSignupConsentWith(func(c *contracts.OAuthSignupConsent) { c.PrivacyVersion = "2025-01-01" }),
			apperrors.CodeTermsAcceptanceRequired,
		},
		"attested age below the minimum": {
			signupconsent.CurrentOAuthSignupConsentWith(func(c *contracts.OAuthSignupConsent) { c.MinAgeAttested = legal.MinSignupAge - 1 }),
			apperrors.CodeAgeConfirmationRequired,
		},
	} {
		s.Run(name, func() {
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

func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_CreationWithConsent_RecordsIt() {
	svc := s.newService()
	consent := signupconsent.CurrentOAuthSignupConsent()

	user, err := svc.FindOrCreateAppleUser(appleClaims("apple-sub-consent", "consent@example.com"), "With", "Consent", consent)
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
	claims := appleClaims("apple-sub-returning-consent", "returning-consent@example.com")
	created, err := svc.FindOrCreateAppleUser(claims, "", "", signupconsent.CurrentOAuthSignupConsent())
	s.Require().NoError(err)
	var before authm.User
	s.Require().NoError(s.db.First(&before, created.ID).Error)

	for name, consent := range map[string]*contracts.OAuthSignupConsent{
		"no consent":    nil,
		"empty consent": {},
		"stale consent": signupconsent.CurrentOAuthSignupConsentWith(func(c *contracts.OAuthSignupConsent) { c.TermsVersion = "2025-01-01" })(),
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

// An Apple account that holds no terms, privacy or age record, as every one
// created before consent was required does, signs in without consent and is
// left without one: the returning branch neither refuses it nor backfills it.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_AccountWithoutLegalRecord_SignsIn() {
	legacy := &authm.User{Email: stringPtr("legacy-apple@example.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(legacy).Error)
	s.Require().NoError(s.db.Create(&authm.OAuthAccount{UserID: legacy.ID, Provider: "apple", ProviderUserID: "apple-sub-legacy"}).Error)

	user, err := s.newService().FindOrCreateAppleUser(appleClaims("apple-sub-legacy", "legacy-apple@example.com"), "", "", nil)

	s.Require().NoError(err)
	s.Equal(legacy.ID, user.ID)
	var stored authm.User
	s.Require().NoError(s.db.First(&stored, legacy.ID).Error)
	s.Nil(stored.TermsVersion)
	s.Nil(stored.PrivacyVersion)
	s.Nil(stored.TermsAcceptedAt)
	s.Nil(stored.AgeConfirmedAt)
	s.Nil(stored.MinAgeAttested)
}

// An address that already belongs to an account refuses the sign-in before the
// consent is consulted, as the Google path does, so a client with no consent
// step is told the address is taken rather than asked for consent it cannot
// use.
func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_HeldAddress_RefusedBeforeConsent() {
	existing := &authm.User{Email: stringPtr("held-consent@example.com"), IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(existing).Error)

	user, err := s.newService().FindOrCreateAppleUser(appleClaims("apple-sub-held-consent", "held-consent@example.com"), "", "", nil)

	s.Require().Error(err)
	s.Nil(user)
	var authErr *apperrors.AuthError
	s.Require().ErrorAs(err, &authErr)
	s.Equal(apperrors.CodeOAuthLinkRefused, authErr.Code)
	s.assertNoAppleAccountFor(existing.ID)
}
