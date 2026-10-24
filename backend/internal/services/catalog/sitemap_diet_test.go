package catalog

import (
	"os"
	"strconv"
	"testing"
)

// TestSitemapDietFromEnvIsOffUnlessSetExactly pins the default-off contract
// and which flag feeds which rule. How each value is parsed is
// shared.EnvPositiveInt's and shared.EnvFlagEnabled's, tested there; this
// covers one malformed value per flag to prove the wiring, and the age limit,
// which is this package's own.
func TestSitemapDietFromEnvIsOffUnlessSetExactly(t *testing.T) {
	keys := []string{sitemapPastShowMaxAgeDaysEnv, sitemapOmitUnlinkedReleasesEnv, sitemapOmitIdleArtistsEnv}
	unsetAll := func(t *testing.T) {
		t.Helper()
		for _, key := range keys {
			// Setenv first so the test framework restores the variable afterwards.
			t.Setenv(key, "")
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("unset %s: %v", key, err)
			}
		}
	}
	cases := []struct {
		name string
		env  map[string]string
		want sitemapDiet
	}{
		{name: "unset is every rule off", want: sitemapDiet{}},
		{name: "malformed age is off", env: map[string]string{sitemapPastShowMaxAgeDaysEnv: "abc"}},
		{name: "zero age is off", env: map[string]string{sitemapPastShowMaxAgeDaysEnv: "0"}},
		{
			name: "age past the limit is off",
			env:  map[string]string{sitemapPastShowMaxAgeDaysEnv: strconv.Itoa(sitemapPastShowMaxAgeDaysLimit + 1)},
		},
		{
			name: "age 30 is only the past-show rule",
			env:  map[string]string{sitemapPastShowMaxAgeDaysEnv: "30"},
			want: sitemapDiet{pastShowMaxAgeDays: 30},
		},
		{
			name: "age pasted with whitespace is on",
			env:  map[string]string{sitemapPastShowMaxAgeDaysEnv: " 30\n"},
			want: sitemapDiet{pastShowMaxAgeDays: 30},
		},
		{
			name: "age at the limit is on",
			env:  map[string]string{sitemapPastShowMaxAgeDaysEnv: strconv.Itoa(sitemapPastShowMaxAgeDaysLimit)},
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
			if got := sitemapDietFromEnv(); got != tc.want {
				t.Errorf("sitemapDietFromEnv() with %v = %+v, want %+v", tc.env, got, tc.want)
			}
		})
	}
}

// TestSitemapDietRulesOffAreTheZeroOmission holds the "off is today's
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
