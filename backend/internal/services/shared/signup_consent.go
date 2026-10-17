package shared

import (
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// minOAuthSignupAge is the minimum age a new OAuth or Apple account must have
// attested to. It equals the auth handlers' MinSignupAge, which this package
// cannot import without a service-to-handler cycle.
const minOAuthSignupAge = 16

// ValidateOAuthSignupConsent refuses an identity-provider sign-up (Google,
// GitHub, Apple) whose consent does not accept the current Terms of Service
// and Privacy Policy with the minimum age attested. A nil consent, an
// unaccepted one, or a version that is not current answers
// CodeTermsAcceptanceRequired; a missing or short age attestation answers
// CodeAgeConfirmationRequired. The consent is caller-supplied on every path
// that reaches this (a cookie, a request body), so this is the check that
// stands where the account is made.
func ValidateOAuthSignupConsent(consent *contracts.OAuthSignupConsent) error {
	if consent == nil || !consent.TermsAccepted {
		return apperrors.ErrTermsAcceptanceRequired("terms acceptance required for OAuth signup")
	}
	if !legal.AcceptsCurrentVersions(consent.TermsVersion, consent.PrivacyVersion) {
		return apperrors.ErrTermsAcceptanceRequired("terms or privacy version is not current for OAuth signup")
	}
	if !consent.AgeConfirmed {
		return apperrors.ErrAgeConfirmationRequired("age confirmation required for OAuth signup")
	}
	if consent.MinAgeAttested < minOAuthSignupAge {
		return apperrors.ErrAgeConfirmationRequired("attested age below minimum for OAuth signup")
	}
	return nil
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
