package shared

import (
	"errors"
	"testing"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

// The consent reaches account creation in a cookie or a request body the
// caller writes, so the check refuses every consent that does not accept the
// current versions with the minimum age attested, on its own.
func TestValidateOAuthSignupConsent(t *testing.T) {
	for name, tc := range map[string]struct {
		consent  func() *contracts.OAuthSignupConsent
		wantCode string
	}{
		"nil": {
			func() *contracts.OAuthSignupConsent { return nil },
			apperrors.CodeTermsAcceptanceRequired,
		},
		"zero value": {
			func() *contracts.OAuthSignupConsent { return &contracts.OAuthSignupConsent{} },
			apperrors.CodeTermsAcceptanceRequired,
		},
		"terms not accepted": {
			mutated(func(c *contracts.OAuthSignupConsent) { c.TermsAccepted = false }),
			apperrors.CodeTermsAcceptanceRequired,
		},
		"terms stale": {
			mutated(func(c *contracts.OAuthSignupConsent) { c.TermsVersion = "2025-01-01" }),
			apperrors.CodeTermsAcceptanceRequired,
		},
		"terms empty": {
			mutated(func(c *contracts.OAuthSignupConsent) { c.TermsVersion = "" }),
			apperrors.CodeTermsAcceptanceRequired,
		},
		"privacy unknown": {
			mutated(func(c *contracts.OAuthSignupConsent) { c.PrivacyVersion = "x" }),
			apperrors.CodeTermsAcceptanceRequired,
		},
		"privacy empty": {
			mutated(func(c *contracts.OAuthSignupConsent) { c.PrivacyVersion = "" }),
			apperrors.CodeTermsAcceptanceRequired,
		},
		"age not confirmed": {
			mutated(func(c *contracts.OAuthSignupConsent) { c.AgeConfirmed = false }),
			apperrors.CodeAgeConfirmationRequired,
		},
		"age below minimum": {
			mutated(func(c *contracts.OAuthSignupConsent) { c.MinAgeAttested = minOAuthSignupAge - 1 }),
			apperrors.CodeAgeConfirmationRequired,
		},
		"age absent": {
			mutated(func(c *contracts.OAuthSignupConsent) { c.MinAgeAttested = 0 }),
			apperrors.CodeAgeConfirmationRequired,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateOAuthSignupConsent(tc.consent())
			var authErr *apperrors.AuthError
			if !errors.As(err, &authErr) || authErr.Code != tc.wantCode {
				t.Fatalf("expected %s, got %v", tc.wantCode, err)
			}
		})
	}

	if err := ValidateOAuthSignupConsent(signupconsent.CurrentOAuthSignupConsent()); err != nil {
		t.Fatalf("current consent refused: %v", err)
	}
}

func mutated(mutate func(*contracts.OAuthSignupConsent)) func() *contracts.OAuthSignupConsent {
	return func() *contracts.OAuthSignupConsent {
		consent := signupconsent.CurrentOAuthSignupConsent()
		mutate(consent)
		return consent
	}
}

func TestStampLegalAcceptance(t *testing.T) {
	t.Run("records every field as given", func(t *testing.T) {
		acceptedAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		ageAt := acceptedAt.Add(time.Minute)
		acceptance := contracts.LegalAcceptance{
			TermsAcceptedAt: acceptedAt,
			TermsVersion:    "terms-v",
			PrivacyVersion:  "privacy-v",
			AgeConfirmedAt:  ageAt,
			MinAgeAttested:  16,
		}
		user := &authm.User{}
		StampLegalAcceptance(user, acceptance)

		if user.TermsAcceptedAt == nil || !user.TermsAcceptedAt.Equal(acceptedAt) {
			t.Errorf("terms_accepted_at = %v, want %v", user.TermsAcceptedAt, acceptedAt)
		}
		if user.TermsVersion == nil || *user.TermsVersion != "terms-v" {
			t.Errorf("terms_version = %v", user.TermsVersion)
		}
		if user.PrivacyVersion == nil || *user.PrivacyVersion != "privacy-v" {
			t.Errorf("privacy_version = %v", user.PrivacyVersion)
		}
		if user.AgeConfirmedAt == nil || !user.AgeConfirmedAt.Equal(ageAt) {
			t.Errorf("age_confirmed_at = %v, want %v", user.AgeConfirmedAt, ageAt)
		}
		if user.MinAgeAttested == nil || *user.MinAgeAttested != 16 {
			t.Errorf("min_age_attested = %v", user.MinAgeAttested)
		}
	})

	t.Run("zero timestamps record one shared now", func(t *testing.T) {
		before := time.Now().UTC()
		user := &authm.User{}
		StampLegalAcceptance(user, contracts.LegalAcceptance{TermsVersion: "t", PrivacyVersion: "p", MinAgeAttested: 16})

		if user.TermsAcceptedAt == nil || user.TermsAcceptedAt.Before(before) {
			t.Fatalf("terms_accepted_at = %v, want at or after %v", user.TermsAcceptedAt, before)
		}
		if user.AgeConfirmedAt == nil || !user.AgeConfirmedAt.Equal(*user.TermsAcceptedAt) {
			t.Errorf("age_confirmed_at = %v, want %v", user.AgeConfirmedAt, *user.TermsAcceptedAt)
		}
	})

	t.Run("no attested age records no age", func(t *testing.T) {
		user := &authm.User{}
		StampLegalAcceptance(user, contracts.LegalAcceptance{TermsVersion: "t", PrivacyVersion: "p"})

		if user.AgeConfirmedAt != nil || user.MinAgeAttested != nil {
			t.Errorf("age recorded without an attestation: %v, %v", user.AgeConfirmedAt, user.MinAgeAttested)
		}
		if user.TermsVersion == nil {
			t.Error("terms not recorded")
		}
	})
}
