package auth

import (
	"net/http"
	"time"

	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	"psychic-homily-backend/internal/services/contracts"
)

// signupAgeRefusalCookieName marks a browser that was refused an account for
// a date of birth under legal.MinSignupAge, on any sign-up path. While it
// lives, every sign-up from that browser is refused for its age whatever date
// it sends, and a new provider identity lands on the refusal rather than on
// the step's form, so a different date cannot be tried straight away. GET
// OAuthPendingSignupRoute reports it as the age_refused status, which both the
// "Create your account" step and the /auth page's Create account tab read.
// The name keeps its oauth_ prefix because it is published in the API and
// renaming it would drop the markers browsers already carry.
const signupAgeRefusalCookieName = "oauth_signup_age_refused"

// SignupAgeRefusalMarkerLifetime is how long the under-age refusal marker
// lives in the browser that was refused. It is a speed bump, not an identity
// check: clearing cookies removes it.
const SignupAgeRefusalMarkerLifetime = 24 * time.Hour

func signupAgeRefusalCookieNameFor(secure bool) string {
	if secure {
		return oauthHostCookiePrefix + signupAgeRefusalCookieName
	}
	return signupAgeRefusalCookieName
}

// signupAgeRefusalMarker reuses only the handshake cookies' attribute builder:
// it outlives oauthHandshakeCookieMaxLifetime by design, so it is not an
// oauthHandshakeCookie.
func signupAgeRefusalMarker(secure bool) http.Cookie {
	return *newOAuthHandshakeCookie(signupAgeRefusalCookieNameFor(secure), "1",
		int(SignupAgeRefusalMarkerLifetime/time.Second), secure)
}

// secureSessionCookies reports whether this deployment sets its cookies
// Secure, which decides the name the refusal marker goes under.
func secureSessionCookies(cfg *config.Config) bool {
	return cfg != nil && cfg.Session.Secure
}

// signupAgeRefusalCookies is the under-age marker as a request carries it,
// under both of the names a deployment may set it under. Only the one
// matching the deployment's Secure setting counts. Each request struct
// declares the two cookie fields itself, because huma binds no parameter
// declared on an embedded struct.
type signupAgeRefusalCookies struct {
	plain, host string
}

func (r *RegisterRequest) ageRefusalCookies() signupAgeRefusalCookies {
	return signupAgeRefusalCookies{r.AgeRefused, r.AgeRefusedHost}
}

func (r *BeginSignupRequest) ageRefusalCookies() signupAgeRefusalCookies {
	return signupAgeRefusalCookies{r.AgeRefused, r.AgeRefusedHost}
}

func (r *FinishSignupRequest) ageRefusalCookies() signupAgeRefusalCookies {
	return signupAgeRefusalCookies{r.AgeRefused, r.AgeRefusedHost}
}

func (c signupAgeRefusalCookies) present(secure bool) bool {
	if secure {
		return c.host != ""
	}
	return c.plain != ""
}

// The refusals for a date of birth that is not a plausible calendar date
// (legal.ParseDateOfBirth), and, on the "Create your account" step, for none
// at all. The sign-up forms show the same two sentences
// (DATE_OF_BIRTH_MISSING_MESSAGE and DATE_OF_BIRTH_INVALID_MESSAGE in
// frontend/app/auth/_components/date-of-birth-field.tsx).
const (
	dateOfBirthMissingMessage = "Enter your date of birth."
	dateOfBirthInvalidMessage = "Enter a real date of birth."
)

// errDateOfBirthImplausible is the refusal for a date of birth that is not a
// plausible calendar date: a field the person can fix.
func errDateOfBirthImplausible() *autherrors.AuthError {
	return autherrors.ErrValidationFailed(dateOfBirthInvalidMessage)
}

// signupAgeOutcome is what checkSignupAge found in a sign-up's date of birth.
type signupAgeOutcome int

const (
	// signupAgeAccepted: a plausible date giving at least legal.MinSignupAge.
	signupAgeAccepted signupAgeOutcome = iota
	// signupAgeMissing: no date was sent.
	signupAgeMissing
	// signupAgeImplausible: the value is not a plausible calendar date.
	signupAgeImplausible
	// signupAgeUnderMinimum: a plausible date giving less than
	// legal.MinSignupAge.
	signupAgeUnderMinimum
)

// signupAge is a date of birth and the age it gives at the instant the
// sign-up submitted it.
type signupAge struct {
	dateOfBirth time.Time
	age         int
	attestedAt  time.Time
}

// checkSignupAge is the one age check every sign-up path makes: it reads
// dateOfBirth (legal.DateOfBirthLayout) as submitted at now, computes the age
// with legal.AgeOn, and holds it against legal.MinSignupAge. The signupAge is
// set only when the outcome is signupAgeAccepted.
func checkSignupAge(dateOfBirth string, now time.Time) (signupAge, signupAgeOutcome) {
	if dateOfBirth == "" {
		return signupAge{}, signupAgeMissing
	}
	dob, err := legal.ParseDateOfBirth(dateOfBirth, now)
	if err != nil {
		return signupAge{}, signupAgeImplausible
	}
	age := legal.AgeOn(dob, now)
	if age < legal.MinSignupAge {
		return signupAge{}, signupAgeUnderMinimum
	}
	return signupAge{dateOfBirth: dob, age: age, attestedAt: now.UTC()}, signupAgeAccepted
}

// signupAgeGate is the age check of a sign-up that sends its date of birth in
// the request (password registration, passkey signup/begin), at now. A browser
// carrying the refusal marker is refused whatever date it sends. Otherwise the
// date goes through checkSignupAge: none at all is refused for its age (an
// older client attests no date), an implausible one with
// errDateOfBirthImplausible, and one under the minimum for its age. setMarker
// reports a refusal that sets the marker again, for this browser's next day.
// The signupAge is set only when refusal is nil.
func signupAgeGate(marked bool, dateOfBirth string, now time.Time) (age signupAge, refusal *autherrors.AuthError, setMarker bool) {
	if marked {
		return signupAge{}, autherrors.ErrAgeConfirmationRequired("browser carries the under-age refusal marker"), true
	}
	age, outcome := checkSignupAge(dateOfBirth, now)
	switch outcome {
	case signupAgeMissing:
		return signupAge{}, autherrors.ErrAgeConfirmationRequired("no date of birth"), false
	case signupAgeImplausible:
		return signupAge{}, errDateOfBirthImplausible(), false
	case signupAgeUnderMinimum:
		return signupAge{}, autherrors.ErrAgeConfirmationRequired("date of birth below the minimum age"), true
	}
	return age, nil, false
}

// legalAcceptance is the acceptance a password or passkey sign-up records:
// the versions as sent, accepted at the instant the age was computed, with the
// computed age as the attested one and the date it came from.
func (a signupAge) legalAcceptance(termsVersion, privacyVersion string) contracts.LegalAcceptance {
	dateOfBirth := a.dateOfBirth
	return contracts.LegalAcceptance{
		TermsAcceptedAt: a.attestedAt,
		TermsVersion:    termsVersion,
		PrivacyVersion:  privacyVersion,
		AgeConfirmedAt:  a.attestedAt,
		MinAgeAttested:  a.age,
		DateOfBirth:     &dateOfBirth,
	}
}

// oauthSignupConsent is the consent the "Create your account" step records,
// under the same rule as legalAcceptance.
func (a signupAge) oauthSignupConsent(termsVersion, privacyVersion string) *contracts.OAuthSignupConsent {
	dateOfBirth := a.dateOfBirth
	return &contracts.OAuthSignupConsent{
		TermsAccepted:  true,
		TermsVersion:   termsVersion,
		PrivacyVersion: privacyVersion,
		AcceptedAt:     a.attestedAt,
		AgeConfirmed:   true,
		MinAgeAttested: a.age,
		DateOfBirth:    &dateOfBirth,
	}
}

// acceptanceAttestsAge reports whether an acceptance a sign-up's first step
// recorded carries a date of birth and an age of at least legal.MinSignupAge.
// It holds for every acceptance legalAcceptance builds.
func acceptanceAttestsAge(acceptance contracts.LegalAcceptance) bool {
	return acceptance.DateOfBirth != nil &&
		!acceptance.AgeConfirmedAt.IsZero() &&
		acceptance.MinAgeAttested >= legal.MinSignupAge
}
