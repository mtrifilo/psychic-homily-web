// Package legal names the versions of the Terms of Service and the Privacy
// Policy that email registration, passkey signup, the Google/GitHub sign-up and
// a Sign in with Apple account creation accept, and the minimum age each of
// them requires. Each of those refuses any other version.
package legal

import "time"

// MinSignupAge is the minimum age (in years) a user must confirm at signup,
// matching the /terms and /privacy minimum-age clauses. The backend is the
// authoritative gate: every account-creation path requires an age confirmation
// of at least this value, independent of the frontend checkbox.
const MinSignupAge = 16

// The versions the frontend's sign-up forms send (CURRENT_TERMS_VERSION and
// CURRENT_PRIVACY_VERSION in frontend/lib/legal.ts).
// TestVersionsMatchFrontend fails when the two files differ.
const (
	CurrentTermsVersion   = "2026-01-31"
	CurrentPrivacyVersion = "2026-10-24"
)

// The grace set. A version bump refuses every sign-up that still sends the
// outgoing value: a browser tab that loaded the form before the frontend
// deployed, a frontend still at the outgoing versions while this backend is
// ahead of it, and an OAuth or passkey sign-up begun before the bump and
// finished after it. Until graceUntil, AcceptedVersions also takes these
// previous versions, either document's alone or both together.
//
// It covers a client behind this backend, never one ahead of it: a version
// this build does not name yet is refused whatever the window, as is every
// sign-up from a frontend that deploys a bump before this backend does. GET
// /legal/versions publishes the versions this build accepts.
//
// They are empty, and graceUntil is the zero time, while no window is open. A
// bump sets both previous versions to the pair the frontend sent before it
// (the outgoing Current values, a document whose version did not change
// included), sets graceUntil to a UTC instant later than the slower of the two
// deploys plus the max-age GET /legal/versions is cached for, and changes
// PREVIOUS_TERMS_VERSION and PREVIOUS_PRIVACY_VERSION in frontend/lib/legal.ts
// to match. TestGraceSetIsCoherent and TestVersionsMatchFrontend pin each of
// those.
const (
	PreviousTermsVersion   = "2026-01-31"
	PreviousPrivacyVersion = "2026-02-15"
)

// graceUntil is the instant the previous versions stop being accepted, in UTC.
var graceUntil = time.Date(2026, time.November, 8, 0, 0, 0, 0, time.UTC)

// Versions is one version of the Terms of Service and one of the Privacy
// Policy.
type Versions struct {
	Terms   string
	Privacy string
}

// versionPolicy is every version pair a sign-up may send: the current pair at
// any instant, and the previous versions in place of either current one before
// graceUntil. A previous version that is empty matches nothing.
type versionPolicy struct {
	current    Versions
	previous   Versions
	graceUntil time.Time
}

func publishedPolicy() versionPolicy {
	return versionPolicy{
		current:    Versions{Terms: CurrentTermsVersion, Privacy: CurrentPrivacyVersion},
		previous:   Versions{Terms: PreviousTermsVersion, Privacy: PreviousPrivacyVersion},
		graceUntil: graceUntil,
	}
}

func (p versionPolicy) graceOpen(now time.Time) bool {
	return now.Before(p.graceUntil)
}

func (p versionPolicy) accepts(termsVersion, privacyVersion string, now time.Time) bool {
	open := p.graceOpen(now)
	return acceptsDocument(termsVersion, p.current.Terms, p.previous.Terms, open) &&
		acceptsDocument(privacyVersion, p.current.Privacy, p.previous.Privacy, open)
}

func acceptsDocument(sent, current, previous string, graceOpen bool) bool {
	if sent == current {
		return true
	}
	return graceOpen && previous != "" && sent == previous
}

// AcceptedVersions reports whether a sign-up at now accepted a Terms of
// Service and a Privacy Policy version that an account may be created with:
// the current one of each, or, before the grace window closes, the previous
// one of either or both. An empty, stale or unknown value for either is
// refused. The comparison is exact; callers store the versions as sent.
func AcceptedVersions(termsVersion, privacyVersion string, now time.Time) bool {
	return publishedPolicy().accepts(termsVersion, privacyVersion, now)
}

// GraceWindow returns the previous versions and the instant they stop being
// accepted, with open reporting whether that instant is after now. When open
// is false the other two results are the zero values, so a closed window that
// still names previous versions is not published as one that accepts them.
func GraceWindow(now time.Time) (previous Versions, until time.Time, open bool) {
	return publishedPolicy().window(now)
}

func (p versionPolicy) window(now time.Time) (Versions, time.Time, bool) {
	if !p.graceOpen(now) {
		return Versions{}, time.Time{}, false
	}
	return p.previous, p.graceUntil, true
}
