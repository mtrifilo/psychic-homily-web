package shared

import (
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// ValidateOAuthSignupConsent refuses an identity-provider sign-up (Google,
// GitHub, Apple) whose consent does not accept a Terms of Service and Privacy
// Policy version that legal.AcceptedVersions takes at the moment of creation,
// with the minimum age attested, and otherwise returns the acceptance the
// account records: both versions as sent, and AcceptedAt as when the terms
// were accepted and the age confirmed. A nil consent, an unaccepted one, or a
// version AcceptedVersions refuses answers CodeTermsAcceptanceRequired; a
// missing or short age attestation answers CodeAgeConfirmationRequired. The
// consent is caller-supplied on every path that reaches this (a cookie, a
// request body), so this is the check that stands where the account is made.
func ValidateOAuthSignupConsent(consent *contracts.OAuthSignupConsent) (contracts.LegalAcceptance, error) {
	if consent == nil || !consent.TermsAccepted {
		return contracts.LegalAcceptance{}, apperrors.ErrTermsAcceptanceRequired("terms acceptance required for OAuth signup")
	}
	if !legal.AcceptedVersions(consent.TermsVersion, consent.PrivacyVersion, time.Now()) {
		return contracts.LegalAcceptance{}, apperrors.ErrTermsAcceptanceRequired("terms or privacy version is not accepted for OAuth signup")
	}
	if !consent.AgeConfirmed {
		return contracts.LegalAcceptance{}, apperrors.ErrAgeConfirmationRequired("age confirmation required for OAuth signup")
	}
	if consent.MinAgeAttested < legal.MinSignupAge {
		return contracts.LegalAcceptance{}, apperrors.ErrAgeConfirmationRequired("attested age below minimum for OAuth signup")
	}
	return contracts.LegalAcceptance{
		TermsAcceptedAt: consent.AcceptedAt,
		TermsVersion:    consent.TermsVersion,
		PrivacyVersion:  consent.PrivacyVersion,
		AgeConfirmedAt:  consent.AcceptedAt,
		MinAgeAttested:  consent.MinAgeAttested,
	}, nil
}

// StampLegalAcceptance writes acceptance onto a user that is about to be
// created: the terms and privacy versions, when the terms were accepted, and,
// when an age was attested (MinAgeAttested > 0), when and to what age. A zero
// timestamp in acceptance is recorded as now, one instant for both.
func StampLegalAcceptance(user *authm.User, acceptance contracts.LegalAcceptance) {
	now := time.Now().UTC()

	acceptedAt := acceptance.TermsAcceptedAt
	if acceptedAt.IsZero() {
		acceptedAt = now
	}
	user.TermsAcceptedAt = &acceptedAt
	user.TermsVersion = &acceptance.TermsVersion
	user.PrivacyVersion = &acceptance.PrivacyVersion

	if acceptance.MinAgeAttested <= 0 {
		return
	}
	ageConfirmedAt := acceptance.AgeConfirmedAt
	if ageConfirmedAt.IsZero() {
		ageConfirmedAt = now
	}
	user.AgeConfirmedAt = &ageConfirmedAt
	user.MinAgeAttested = &acceptance.MinAgeAttested
}
