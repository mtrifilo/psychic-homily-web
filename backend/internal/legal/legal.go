// Package legal names the versions of the Terms of Service and the Privacy
// Policy that email registration, passkey signup, the Google/GitHub sign-up and
// a Sign in with Apple account creation accept. Each of those refuses any other
// version.
package legal

// The versions the frontend's sign-up forms send (CURRENT_TERMS_VERSION and
// CURRENT_PRIVACY_VERSION in frontend/lib/legal.ts).
// TestCurrentVersionsMatchFrontend fails when the two files differ.
//
// Exactly one value of each is accepted, so a bump refuses every sign-up from
// a frontend that still sends the old value: a browser tab that loaded the
// form before the frontend deployed, and every client while the backend and
// the frontend are deployed at different values.
const (
	CurrentTermsVersion   = "2026-01-31"
	CurrentPrivacyVersion = "2026-02-15"
)

// AcceptsCurrentVersions reports whether a sign-up accepted the current Terms
// of Service and the current Privacy Policy. An empty, stale or unknown value
// for either is not an acceptance of the current document.
func AcceptsCurrentVersions(termsVersion, privacyVersion string) bool {
	return termsVersion == CurrentTermsVersion && privacyVersion == CurrentPrivacyVersion
}
