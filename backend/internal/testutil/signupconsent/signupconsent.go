// Package signupconsent builds the consent a sign-up carries, for tests that
// create accounts through the services. It is apart from testutil because it
// imports contracts, which imports models whose tests import testutil.
package signupconsent

import (
	"time"

	"psychic-homily-backend/internal/legal"
	"psychic-homily-backend/internal/services/contracts"
)

// signupMinimumAge is the minimum age a sign-up attests.
const signupMinimumAge = legal.MinSignupAge

// CurrentLegalAcceptance is a password or passkey sign-up's acceptance of the
// current terms and privacy versions with the minimum age attested.
func CurrentLegalAcceptance() contracts.LegalAcceptance {
	now := time.Now().UTC()
	return contracts.LegalAcceptance{
		TermsAcceptedAt: now,
		TermsVersion:    legal.CurrentTermsVersion,
		PrivacyVersion:  legal.CurrentPrivacyVersion,
		AgeConfirmedAt:  now,
		MinAgeAttested:  signupMinimumAge,
	}
}

// CurrentOAuthSignupConsent is an OAuth sign-up's consent to the current terms
// and privacy versions with the minimum age attested. Each call returns a new
// value, so a caller may change it without affecting another.
func CurrentOAuthSignupConsent() *contracts.OAuthSignupConsent {
	return &contracts.OAuthSignupConsent{
		TermsAccepted:  true,
		TermsVersion:   legal.CurrentTermsVersion,
		PrivacyVersion: legal.CurrentPrivacyVersion,
		AcceptedAt:     time.Now().UTC(),
		AgeConfirmed:   true,
		MinAgeAttested: signupMinimumAge,
	}
}

// AcceptedVersionPairs names every version pair a sign-up at now must be
// accepted with and recorded as sent: the current pair, and, while
// legal.GraceWindow reports a window open, the previous pair and both mixed
// pairs. With no window configured it is the current pair alone, so a sweep
// built on it reaches the grace window on the build that opens one.
func AcceptedVersionPairs(now time.Time) map[string]legal.Versions {
	current := legal.Versions{Terms: legal.CurrentTermsVersion, Privacy: legal.CurrentPrivacyVersion}
	pairs := map[string]legal.Versions{"current pair": current}
	previous, _, open := legal.GraceWindow(now)
	if !open {
		return pairs
	}
	pairs["previous pair"] = previous
	pairs["current terms with previous privacy"] = legal.Versions{Terms: current.Terms, Privacy: previous.Privacy}
	pairs["previous terms with current privacy"] = legal.Versions{Terms: previous.Terms, Privacy: current.Privacy}
	return pairs
}

// CurrentOAuthSignupConsentWith is CurrentOAuthSignupConsent changed by
// mutate, built anew on each call of the returned function.
func CurrentOAuthSignupConsentWith(mutate func(*contracts.OAuthSignupConsent)) func() *contracts.OAuthSignupConsent {
	return func() *contracts.OAuthSignupConsent {
		consent := CurrentOAuthSignupConsent()
		mutate(consent)
		return consent
	}
}
