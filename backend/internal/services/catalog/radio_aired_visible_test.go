package catalog

import (
	"strings"
	"testing"
)

// TestAiredEpisodeVisibleSQL pins the shared air-window predicate's shape (PSY-1285):
// exactly one bind placeholder (the "now" instant) and the prefix interpolated onto
// every column, so the call sites can keep binding exactly one arg and qualify columns
// correctly in a joined query. A drift here (an extra ?, or a dropped prefix) would
// silently misbind or reference the wrong table.
func TestAiredEpisodeVisibleSQL(t *testing.T) {
	for _, prefix := range []string{"", "re."} {
		got := airedEpisodeVisibleSQL(prefix)
		if n := strings.Count(got, "?"); n != 1 {
			t.Errorf("airedEpisodeVisibleSQL(%q): want exactly 1 bind placeholder, got %d in %q", prefix, n, got)
		}
		if !strings.Contains(got, prefix+"starts_at") || !strings.Contains(got, prefix+"play_count") {
			t.Errorf("airedEpisodeVisibleSQL(%q): missing prefix-qualified columns in %q", prefix, got)
		}
		// For a non-empty prefix, every column reference must carry it — a bare column
		// would be ambiguous in the joined feed query (radio_episodes + radio_shows +
		// radio_stations). (The check is vacuous for the empty prefix, so only assert it
		// when a prefix is set.)
		if prefix != "" && (strings.Count(got, "starts_at") != strings.Count(got, prefix+"starts_at") ||
			strings.Count(got, "play_count") != strings.Count(got, prefix+"play_count")) {
			t.Errorf("airedEpisodeVisibleSQL(%q): an unqualified column slipped through: %q", prefix, got)
		}
	}
}
