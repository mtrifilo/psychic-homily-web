package seeddata

import (
	"strings"
	"testing"
)

// TestRenderRadioSeedSQL_Shape checks the structural properties of the
// generated SQL. We don't run it through a real Postgres here (that's
// covered by the E2E suite indirectly), but we guard against regressions
// in the shape: idempotency, NULL handling for empty optionals, and
// quote escaping.
func TestRenderRadioSeedSQL_Shape(t *testing.T) {
	var sb strings.Builder
	if err := RenderRadioSeedSQL(&sb); err != nil {
		t.Fatalf("RenderRadioSeedSQL: %v", err)
	}
	sql := sb.String()

	mustContain := []string{
		"INSERT INTO radio_networks",
		"INSERT INTO radio_stations",
		"INSERT INTO radio_shows",
		"ON CONFLICT (slug) DO NOTHING",
		// station_id FK resolved by slug subquery
		"(SELECT id FROM radio_stations WHERE slug = ",
		// network_id FK resolved by network slug subquery (PSY-508)
		"(SELECT id FROM radio_networks WHERE slug = 'wfmu')",
		// escape: KEXP's -> KEXP''s (apostrophe doubling)
		"KEXP''s flagship morning program",
		// NULL for empty-string state on NTS (UK, no state)
		"'London', NULL, 'GB'",
		// NULL for zero FrequencyMHz on NTS (internet-only)
		"'internet', NULL, 'nts_api'",
		// NULL for empty HostName on "The NTS Breakfast Show" (rotating hosts)
		"'The NTS Breakfast Show', 'breakfast-show-nts', NULL,",
		// PSY-1077: NULL HostName on host-named NTS residencies (host would
		// duplicate the show name, rendering "Floating Points w/ Floating Points")
		"'Floating Points', 'floating-points-nts', NULL,",
		"'Anu', 'anu-nts', NULL,",
		// PSY-508: WFMU sub-stream slugs and apostrophe escapes
		"'wfmu-drummer'",
		"'Rock''n''Soul Radio'",
		"'Sheena''s Jungle Room'",
		// PSY-899: episode + play fixtures
		"INSERT INTO radio_episodes",
		"INSERT INTO radio_plays",
		// episode show_id FK resolved by show slug subquery
		"(SELECT id FROM radio_shows WHERE slug = 'the-morning-show')",
		// episode dedup ON CONFLICT target matches idx_radio_episodes_unique
		"ON CONFLICT (show_id, air_date, COALESCE(external_id, '')) DO NOTHING",
		// play episode_id FK resolved by parent episode's (show_id, air_date)
		"AND air_date = '2025-01-15'",
		// matched play: artist_id resolved from seeded artist slug
		"(SELECT id FROM artists WHERE slug = 'calexico')",
		// play dedup ON CONFLICT target matches idx_radio_plays_unique
		"ON CONFLICT (episode_id, position, air_timestamp, artist_name, track_title) DO NOTHING",
	}
	for _, want := range mustContain {
		if !strings.Contains(sql, want) {
			t.Errorf("generated SQL missing expected substring:\n  want: %q", want)
		}
	}

	// Five ON CONFLICT clauses: networks + stations + shows + episodes + plays.
	if got := strings.Count(sql, "ON CONFLICT"); got != 5 {
		t.Errorf("want 5 ON CONFLICT clauses (networks + stations + shows + episodes + plays), got %d", got)
	}

	// PSY-899: at least one play must be unmatched (artist_id NULL) so the
	// generator covers the common source-metadata-only case. The matched
	// play's artist_id is a subquery, so a bare ", NULL, " in the plays
	// VALUES list is the unmatched signal. Guard the marker lookup so a
	// missing plays INSERT fails cleanly here instead of panicking on a
	// negative slice index (the mustContain loop above uses Errorf, not
	// Fatalf, so execution reaches this point even when the marker is gone).
	playsStart := strings.Index(sql, "INSERT INTO radio_plays")
	if playsStart < 0 {
		t.Fatalf("plays INSERT not found in generated SQL")
	}
	playsSection := sql[playsStart:]
	if !strings.Contains(playsSection, "'Beach House', 'Space Song', NULL,") {
		t.Errorf("expected an unmatched play (artist_id NULL) in the plays VALUES list")
	}
}

func TestSqlString_EscapesApostrophes(t *testing.T) {
	cases := map[string]string{
		"hello":           "'hello'",
		"KEXP's flagship": "'KEXP''s flagship'",
		"don't":           "'don''t'",
		"o'b'r'ien":       "'o''b''r''ien'",
		"":                "''",
	}
	for in, want := range cases {
		if got := sqlString(in); got != want {
			t.Errorf("sqlString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSqlStringOrNull(t *testing.T) {
	if got := sqlStringOrNull(""); got != "NULL" {
		t.Errorf("sqlStringOrNull(\"\") = %q, want NULL", got)
	}
	if got := sqlStringOrNull("WA"); got != "'WA'" {
		t.Errorf("sqlStringOrNull(\"WA\") = %q, want 'WA'", got)
	}
}

func TestSqlFloatOrNull(t *testing.T) {
	if got := sqlFloatOrNull(0); got != "NULL" {
		t.Errorf("sqlFloatOrNull(0) = %q, want NULL", got)
	}
	if got := sqlFloatOrNull(90.3); got != "90.3" {
		t.Errorf("sqlFloatOrNull(90.3) = %q, want 90.3", got)
	}
	if got := sqlFloatOrNull(91.1); got != "91.1" {
		t.Errorf("sqlFloatOrNull(91.1) = %q, want 91.1", got)
	}
}

// TestRadioShows_NoHostNameDuplicatingShowName guards PSY-1077: host-named
// residencies (common on NTS) must seed HostName empty (-> NULL) rather than
// repeating the show name, which renders "Floating Points w/ Floating Points"
// on now-playing surfaces. Both seed consumers (cmd/seed via GORM and
// RenderRadioSeedSQL via cmd/gen-e2e-seed) read these structs, so the
// invariant holds for dev, stage, and E2E databases alike.
func TestRadioShows_NoHostNameDuplicatingShowName(t *testing.T) {
	for _, s := range RadioShows {
		if s.HostName != "" && strings.EqualFold(s.HostName, s.Name) {
			t.Errorf("show %q (slug %q): HostName duplicates the show name; leave it empty so it seeds as NULL (PSY-1077)", s.Name, s.Slug)
		}
	}
}

// TestRenderRadioSeedSQL_RowCounts guards against accidentally shipping
// a partial data set: the generator should emit exactly as many
// INSERT-value rows as the package-level slices declare.
func TestRenderRadioSeedSQL_RowCounts(t *testing.T) {
	var sb strings.Builder
	if err := RenderRadioSeedSQL(&sb); err != nil {
		t.Fatalf("RenderRadioSeedSQL: %v", err)
	}
	sql := sb.String()

	// Each row ends with "NOW(), NOW()),". The final row of each table
	// ends with "NOW(), NOW())" (no comma) followed by ON CONFLICT.
	// Cheap count: commas after the NOW(), NOW()) pattern + 3 non-comma
	// terminals (one per table: networks + stations + shows).
	rows := strings.Count(sql, "NOW(), NOW()),") + 3
	want := len(RadioNetworks) + len(RadioStations) + len(RadioShows)
	if rows != want {
		t.Errorf("row count mismatch: got %d, want %d (networks=%d + stations=%d + shows=%d)",
			rows, want, len(RadioNetworks), len(RadioStations), len(RadioShows))
	}
}
