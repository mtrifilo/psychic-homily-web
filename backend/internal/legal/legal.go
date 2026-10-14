// Package legal names the versions of the Terms of Service and the Privacy
// Policy a new account accepts. Every sign-up path refuses any other version,
// so these are the only values a new users row records.
package legal

// The versions the frontend's sign-up forms send (CURRENT_TERMS_VERSION and
// CURRENT_PRIVACY_VERSION in frontend/lib/legal.ts). A sign-up carrying any
// other value is refused, so a version bump changes both files together;
// TestCurrentVersionsMatchFrontend fails when they differ.
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
