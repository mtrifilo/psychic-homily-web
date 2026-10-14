package legal

import (
	"os"
	"regexp"
	"testing"
)

// frontendLegalPath is the module the frontend's sign-up forms read their
// version strings from. Relative to this package's directory.
const frontendLegalPath = "../../../frontend/lib/legal.ts"

// TestCurrentVersionsMatchFrontend pins the versions the backend accepts to
// the ones the frontend sends. A frontend that sends a version this package
// does not name is refused on every sign-up path, so a mismatch here is a
// sign-up outage on deploy.
func TestCurrentVersionsMatchFrontend(t *testing.T) {
	source, err := os.ReadFile(frontendLegalPath)
	if err != nil {
		t.Fatalf("cannot read %s: %v\n"+
			"This test pins the backend's accepted legal versions to the frontend's. "+
			"If that module moved, repoint this constant; do not delete the pin.",
			frontendLegalPath, err)
	}

	for _, pin := range []struct {
		frontendName string
		backend      string
	}{
		{"CURRENT_TERMS_VERSION", CurrentTermsVersion},
		{"CURRENT_PRIVACY_VERSION", CurrentPrivacyVersion},
	} {
		pattern := regexp.MustCompile(`(?m)^export const ` + pin.frontendName + `\s*=\s*'([^']*)'`)
		match := pattern.FindSubmatch(source)
		if match == nil {
			t.Errorf("no %s string constant found in %s", pin.frontendName, frontendLegalPath)
			continue
		}
		if frontend := string(match[1]); frontend != pin.backend {
			t.Errorf("%s differs: frontend sends %q, backend accepts %q", pin.frontendName, frontend, pin.backend)
		}
	}
}

func TestAcceptsCurrentVersions(t *testing.T) {
	for _, tc := range []struct {
		name           string
		terms, privacy string
		want           bool
	}{
		{"current", CurrentTermsVersion, CurrentPrivacyVersion, true},
		{"terms empty", "", CurrentPrivacyVersion, false},
		{"privacy empty", CurrentTermsVersion, "", false},
		{"terms stale", "2025-01-01", CurrentPrivacyVersion, false},
		{"privacy unknown", CurrentTermsVersion, "x", false},
		{"swapped", CurrentPrivacyVersion, CurrentTermsVersion, false},
		{"padded", " " + CurrentTermsVersion, CurrentPrivacyVersion, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := AcceptsCurrentVersions(tc.terms, tc.privacy); got != tc.want {
				t.Errorf("AcceptsCurrentVersions(%q, %q) = %v, want %v", tc.terms, tc.privacy, got, tc.want)
			}
		})
	}
}
