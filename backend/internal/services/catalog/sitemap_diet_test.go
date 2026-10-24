package catalog

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"psychic-homily-backend/internal/testlog"
)

// sitemapDietEnvKeys is every diet flag, for tests that must start with all of
// them unset.
var sitemapDietEnvKeys = []string{
	sitemapPastShowMaxAgeDaysEnv,
	sitemapOmitUnlinkedReleasesEnv,
	sitemapOmitIdleArtistsEnv,
}

// TestSitemapDietFromEnvIsOffUnlessSetExactly pins the default-off contract,
// which flag feeds which rule, and which values warn. How each value is parsed
// is shared.EnvPositiveInt's and shared.EnvFlagEnabled's, tested there; this
// covers one malformed value per flag to prove the wiring, and the age limit,
// which is this package's own.
func TestSitemapDietFromEnvIsOffUnlessSetExactly(t *testing.T) {
	unsetAll := func(t *testing.T) {
		t.Helper()
		for _, key := range sitemapDietEnvKeys {
			// Setenv first so the test framework restores the variable afterwards.
			t.Setenv(key, "")
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("unset %s: %v", key, err)
			}
		}
	}
	age := func(v string) map[string]string { return map[string]string{sitemapPastShowMaxAgeDaysEnv: v} }
	cases := []struct {
		name     string
		env      map[string]string
		want     sitemapDiet
		wantWarn bool
	}{
		{name: "unset is every rule off"},
		{name: "zero age is off, silently", env: age("0")},
		{name: "zero spelled 00 is off, silently", env: age("00")},
		{name: "malformed age is off, with a warning", env: age("abc"), wantWarn: true},
		{name: "negative age is off, with a warning", env: age("-5"), wantWarn: true},
		{name: "age past the limit is off, with a warning", env: age(strconv.Itoa(sitemapPastShowMaxAgeDaysLimit + 1)), wantWarn: true},
		{name: "age too large to parse is off, with a warning", env: age("99999999999999999999"), wantWarn: true},
		{name: "age 30 is only the past-show rule", env: age("30"), want: sitemapDiet{pastShowMaxAgeDays: 30}},
		{name: "age pasted with whitespace is on", env: age(" 30\n"), want: sitemapDiet{pastShowMaxAgeDays: 30}},
		{
			name: "age at the limit is on",
			env:  age(strconv.Itoa(sitemapPastShowMaxAgeDaysLimit)),
			want: sitemapDiet{pastShowMaxAgeDays: sitemapPastShowMaxAgeDaysLimit},
		},
		{
			name: "true is not a boolean flag's on",
			env:  map[string]string{sitemapOmitUnlinkedReleasesEnv: "true", sitemapOmitIdleArtistsEnv: "true"},
		},
		{
			name: "unlinked releases 1 is only that rule",
			env:  map[string]string{sitemapOmitUnlinkedReleasesEnv: "1"},
			want: sitemapDiet{omitUnlinkedReleases: true},
		},
		{
			name: "idle artists 1 is only that rule",
			env:  map[string]string{sitemapOmitIdleArtistsEnv: "1"},
			want: sitemapDiet{omitIdleArtists: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			unsetAll(t)
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			var got sitemapDiet
			// The warning cases are the positive marker that Capture reaches
			// sitemapDietFromEnv's logger, which the silent cases rely on.
			out := testlog.Capture(t, func() { got = sitemapDietFromEnv() })
			if got != tc.want {
				t.Errorf("sitemapDietFromEnv() with %v = %+v, want %+v", tc.env, got, tc.want)
			}
			warned := strings.Contains(out, "level=WARN") && strings.Contains(out, sitemapPastShowMaxAgeDaysEnv)
			if warned != tc.wantWarn {
				t.Errorf("sitemapDietFromEnv() with %v warned = %v, want %v (log: %q)", tc.env, warned, tc.wantWarn, out)
			}
		})
	}
}

// TestSitemapDietRulesOffAreTheZeroOmission holds the "off is entriesFor's own
// statement" property at its source: a rule that is off hands entriesOmitting
// the zero omission, whose empty condition delegates to entriesFor.
func TestSitemapDietRulesOffAreTheZeroOmission(t *testing.T) {
	off := sitemapDiet{}
	for name, o := range map[string]sitemapOmission{
		"pastShows":        off.pastShows(),
		"unlinkedReleases": off.unlinkedReleases(),
		"idleArtists":      off.idleArtists(),
	} {
		if o != (sitemapOmission{}) {
			t.Errorf("%s with the rule off = %+v, want the zero omission", name, o)
		}
	}
}
