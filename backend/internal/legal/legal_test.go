package legal

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// frontendLegalPath is the module the frontend's sign-up forms read their
// version strings from. Relative to this package's directory.
const frontendLegalPath = "../../../frontend/lib/legal.ts"

// TestVersionsMatchFrontend pins the versions the backend accepts to the ones
// the frontend sends and knows about. A frontend that sends a version this
// package does not name is refused by every sign-up that takes consent, so a
// mismatch here is a sign-up outage on deploy. The previous versions are pinned
// empty or not, so the two files move together on every bump and every close.
func TestVersionsMatchFrontend(t *testing.T) {
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
		{"PREVIOUS_TERMS_VERSION", PreviousTermsVersion},
		{"PREVIOUS_PRIVACY_VERSION", PreviousPrivacyVersion},
	} {
		pattern := regexp.MustCompile(`(?m)^export const ` + pin.frontendName + `\s*=\s*'([^']*)'`)
		match := pattern.FindSubmatch(source)
		if match == nil {
			t.Errorf("no %s string constant found in %s", pin.frontendName, frontendLegalPath)
			continue
		}
		if frontend := string(match[1]); frontend != pin.backend {
			t.Errorf("%s differs: frontend has %q, backend has %q", pin.frontendName, frontend, pin.backend)
		}
	}

	age := regexp.MustCompile(`(?m)^export const MIN_SIGNUP_AGE\s*=\s*(\d+)\s*$`).FindSubmatch(source)
	if age == nil {
		t.Fatalf("no MIN_SIGNUP_AGE number constant found in %s", frontendLegalPath)
	}
	if frontend, _ := strconv.Atoi(string(age[1])); frontend != MinSignupAge {
		t.Errorf("MIN_SIGNUP_AGE differs: frontend has %d, backend has %d", frontend, MinSignupAge)
	}
}

// TestGraceSetIsCoherent pins the shape a bump must leave the grace set in. A
// window with one previous version empty would refuse the pre-bump pair it
// exists to accept; previous versions with no window are dead configuration
// the frontend would still be told about; a non-UTC instant reads differently
// in every deploy's local zone.
func TestGraceSetIsCoherent(t *testing.T) {
	if CurrentTermsVersion == "" || CurrentPrivacyVersion == "" {
		t.Fatal("a current version is empty: AcceptedVersions would accept an empty value")
	}
	if graceUntil.IsZero() {
		if PreviousTermsVersion != "" || PreviousPrivacyVersion != "" {
			t.Errorf("previous versions (%q, %q) are set with no grace window: set graceUntil, or clear them",
				PreviousTermsVersion, PreviousPrivacyVersion)
		}
		return
	}
	if PreviousTermsVersion == "" || PreviousPrivacyVersion == "" {
		t.Errorf("grace window set with a previous version empty (%q, %q): both must be the pair the frontend sent before the bump",
			PreviousTermsVersion, PreviousPrivacyVersion)
	}
	if PreviousTermsVersion == CurrentTermsVersion && PreviousPrivacyVersion == CurrentPrivacyVersion {
		t.Error("grace window set but the previous pair equals the current pair: nothing was bumped")
	}
	if graceUntil.Location() != time.UTC {
		t.Errorf("graceUntil is in %s, want UTC", graceUntil.Location())
	}
}

func TestVersionPolicyAccepts(t *testing.T) {
	const (
		termsOld, termsNew     = "2026-01-31", "2026-10-01"
		privacyOld, privacyNew = "2026-02-15", "2026-10-02"
	)
	bump := time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)
	until := bump.Add(72 * time.Hour)

	bothBumped := versionPolicy{
		current:    Versions{Terms: termsNew, Privacy: privacyNew},
		previous:   Versions{Terms: termsOld, Privacy: privacyOld},
		graceUntil: until,
	}
	termsOnlyBumped := versionPolicy{
		current:    Versions{Terms: termsNew, Privacy: privacyOld},
		previous:   Versions{Terms: termsOld, Privacy: privacyOld},
		graceUntil: until,
	}
	noBump := versionPolicy{current: Versions{Terms: termsNew, Privacy: privacyNew}}
	previousWithoutWindow := versionPolicy{
		current:  Versions{Terms: termsNew, Privacy: privacyNew},
		previous: Versions{Terms: termsOld, Privacy: privacyOld},
	}
	windowWithoutPrevious := versionPolicy{
		current:    Versions{Terms: termsNew, Privacy: privacyNew},
		graceUntil: until,
	}

	instants := []struct {
		name string
		at   time.Time
		open bool
	}{
		{"long before the window closes", bump.Add(-365 * 24 * time.Hour), true},
		{"one second after the bump", bump.Add(time.Second), true},
		{"last instant of the window", until.Add(-time.Nanosecond), true},
		{"the instant the window closes", until, false},
		{"one second after the window", until.Add(time.Second), false},
	}

	type pair struct{ terms, privacy string }
	current := pair{termsNew, privacyNew}
	insideOnly := map[string]pair{
		"previous pair":              {termsOld, privacyOld},
		"new terms with old privacy": {termsNew, privacyOld},
		"old terms with new privacy": {termsOld, privacyNew},
	}
	alwaysRefused := map[string]pair{
		"terms empty":               {"", privacyNew},
		"privacy empty":             {termsNew, ""},
		"both empty":                {"", ""},
		"terms older than previous": {"2025-01-01", privacyNew},
		"privacy unknown":           {termsNew, "x"},
		"swapped current":           {privacyNew, termsNew},
		"swapped previous":          {privacyOld, termsOld},
		"previous terms padded":     {" " + termsOld, privacyNew},
		"current terms padded":      {termsNew + " ", privacyNew},
	}

	for _, in := range instants {
		t.Run(in.name, func(t *testing.T) {
			if !bothBumped.accepts(current.terms, current.privacy, in.at) {
				t.Error("current pair refused")
			}
			for name, p := range insideOnly {
				if got := bothBumped.accepts(p.terms, p.privacy, in.at); got != in.open {
					t.Errorf("%s (%q, %q) accepted = %v, want %v", name, p.terms, p.privacy, got, in.open)
				}
			}
			for name, p := range alwaysRefused {
				if bothBumped.accepts(p.terms, p.privacy, in.at) {
					t.Errorf("%s (%q, %q) accepted", name, p.terms, p.privacy)
				}
			}

			// A bump of one document: the pre-bump pair repeats the unchanged
			// version, and that version is current, so only the terms window
			// moves.
			if got := termsOnlyBumped.accepts(termsOld, privacyOld, in.at); got != in.open {
				t.Errorf("terms-only bump: pre-bump pair accepted = %v, want %v", got, in.open)
			}
			if !termsOnlyBumped.accepts(termsNew, privacyOld, in.at) {
				t.Error("terms-only bump: current pair refused")
			}
			if termsOnlyBumped.accepts(termsNew, privacyNew, in.at) {
				t.Error("terms-only bump: a privacy version nobody published was accepted")
			}

			for name, policy := range map[string]versionPolicy{
				"no bump":                 noBump,
				"previous without window": previousWithoutWindow,
				"window without previous": windowWithoutPrevious,
			} {
				if !policy.accepts(current.terms, current.privacy, in.at) {
					t.Errorf("%s: current pair refused", name)
				}
				for pairName, p := range insideOnly {
					if policy.accepts(p.terms, p.privacy, in.at) {
						t.Errorf("%s: %s (%q, %q) accepted", name, pairName, p.terms, p.privacy)
					}
				}
				for pairName, p := range alwaysRefused {
					if policy.accepts(p.terms, p.privacy, in.at) {
						t.Errorf("%s: %s (%q, %q) accepted", name, pairName, p.terms, p.privacy)
					}
				}
			}
		})
	}
}

func TestVersionPolicyWindow(t *testing.T) {
	until := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
	previous := Versions{Terms: "2026-01-31", Privacy: "2026-02-15"}
	policy := versionPolicy{
		current:    Versions{Terms: "2026-10-01", Privacy: "2026-10-02"},
		previous:   previous,
		graceUntil: until,
	}

	gotPrevious, gotUntil, open := policy.window(until.Add(-time.Second))
	if !open || gotPrevious != previous || !gotUntil.Equal(until) {
		t.Errorf("inside the window: window = (%+v, %v, %v), want (%+v, %v, true)", gotPrevious, gotUntil, open, previous, until)
	}

	for name, at := range map[string]time.Time{
		"the instant it closes": until,
		"after it closes":       until.Add(time.Hour),
	} {
		gotPrevious, gotUntil, open := policy.window(at)
		if open || gotPrevious != (Versions{}) || !gotUntil.IsZero() {
			t.Errorf("%s: window = (%+v, %v, %v), want the zero values", name, gotPrevious, gotUntil, open)
		}
	}

	noBump := versionPolicy{current: policy.current, previous: previous}
	if gotPrevious, gotUntil, open := noBump.window(until); open || gotPrevious != (Versions{}) || !gotUntil.IsZero() {
		t.Errorf("no window: window = (%+v, %v, %v), want the zero values", gotPrevious, gotUntil, open)
	}
}

// TestPublishedPolicy checks AcceptedVersions and GraceWindow against the
// configuration this package ships, whatever it is: with no window, the
// current pair only; with one, the previous pair up to its last instant.
func TestPublishedPolicy(t *testing.T) {
	now := time.Now()
	if !AcceptedVersions(CurrentTermsVersion, CurrentPrivacyVersion, now) {
		t.Fatal("the current pair is refused")
	}
	if AcceptedVersions("", "", now) {
		t.Fatal("an empty pair is accepted")
	}

	if graceUntil.IsZero() {
		if _, _, open := GraceWindow(now); open {
			t.Error("GraceWindow reports a window none is configured for")
		}
		return
	}

	lastInstant := graceUntil.Add(-time.Nanosecond)
	if !AcceptedVersions(PreviousTermsVersion, PreviousPrivacyVersion, lastInstant) {
		t.Error("the previous pair is refused inside the window")
	}
	if AcceptedVersions(PreviousTermsVersion, PreviousPrivacyVersion, graceUntil) {
		t.Error("the previous pair is accepted once the window closes")
	}
	previous, until, open := GraceWindow(lastInstant)
	if !open || previous != (Versions{Terms: PreviousTermsVersion, Privacy: PreviousPrivacyVersion}) || !until.Equal(graceUntil) {
		t.Errorf("GraceWindow inside the window = (%+v, %v, %v)", previous, until, open)
	}
}
