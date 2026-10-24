package catalog

import (
	"os"
	"strconv"
	"testing"
)

// TestSitemapDietFromEnvIsOffUnlessSetExactly pins the default-off contract:
// an unset, zero, negative, malformed or out-of-range value leaves a rule off,
// and only the documented spellings switch one on.
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

	t.Run("unset is every rule off", func(t *testing.T) {
		unsetAll(t)
		if got := sitemapDietFromEnv(); got != (sitemapDiet{}) {
			t.Errorf("sitemapDietFromEnv() with nothing set = %+v, want every rule off", got)
		}
	})

	for _, value := range []string{
		"", "0", "-30", "abc", "30.5", "30 days", " 30",
		strconv.Itoa(sitemapPastShowMaxAgeDaysLimit + 1), "99999999999999999999",
	} {
		t.Run("past-show age "+strconv.Quote(value)+" is off", func(t *testing.T) {
			unsetAll(t)
			t.Setenv(sitemapPastShowMaxAgeDaysEnv, value)
			if got := sitemapDietFromEnv().pastShowMaxAgeDays; got != 0 {
				t.Errorf("%s=%q read as %d days, want 0 (off)", sitemapPastShowMaxAgeDaysEnv, value, got)
			}
		})
	}

	t.Run("past-show age 30 is on", func(t *testing.T) {
		unsetAll(t)
		t.Setenv(sitemapPastShowMaxAgeDaysEnv, "30")
		if got := sitemapDietFromEnv(); got != (sitemapDiet{pastShowMaxAgeDays: 30}) {
			t.Errorf("sitemapDietFromEnv() = %+v, want only the past-show rule on at 30 days", got)
		}
	})

	for _, value := range []string{"", "0", "true", "yes", "on", "2"} {
		t.Run("boolean flags "+strconv.Quote(value)+" are off", func(t *testing.T) {
			unsetAll(t)
			t.Setenv(sitemapOmitUnlinkedReleasesEnv, value)
			t.Setenv(sitemapOmitIdleArtistsEnv, value)
			if got := sitemapDietFromEnv(); got != (sitemapDiet{}) {
				t.Errorf("flags set to %q read as %+v, want every rule off", value, got)
			}
		})
	}

	t.Run("boolean flags 1 are on, independently", func(t *testing.T) {
		unsetAll(t)
		t.Setenv(sitemapOmitUnlinkedReleasesEnv, "1")
		if got := sitemapDietFromEnv(); got != (sitemapDiet{omitUnlinkedReleases: true}) {
			t.Errorf("only %s=1 read as %+v", sitemapOmitUnlinkedReleasesEnv, got)
		}
		unsetAll(t)
		t.Setenv(sitemapOmitIdleArtistsEnv, "1")
		if got := sitemapDietFromEnv(); got != (sitemapDiet{omitIdleArtists: true}) {
			t.Errorf("only %s=1 read as %+v", sitemapOmitIdleArtistsEnv, got)
		}
	})
}

// TestSitemapDietRulesOffRenderNoCondition holds the "off is today's
// statement" property at its source: a rule that is off hands entriesOmitting
// an empty condition, which is the branch that delegates to entriesFor.
func TestSitemapDietRulesOffRenderNoCondition(t *testing.T) {
	off := sitemapDiet{}
	for name, o := range map[string]sitemapOmission{
		"pastShows":        off.pastShows(),
		"unlinkedReleases": off.unlinkedReleases(),
		"idleArtists":      off.idleArtists(),
	} {
		if o.condition != "" {
			t.Errorf("%s with the rule off renders %q, want no condition", name, o.condition)
		}
		if o.rule == "" {
			t.Errorf("%s names no rule", name)
		}
	}
}
