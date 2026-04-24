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
		"INSERT INTO radio_stations",
		"INSERT INTO radio_shows",
		"ON CONFLICT (slug) DO NOTHING",
		// station_id FK resolved by slug subquery
		"(SELECT id FROM radio_stations WHERE slug = ",
		// escape: KEXP's -> KEXP''s (apostrophe doubling)
		"KEXP''s flagship morning program",
		// NULL for empty-string state on NTS (UK, no state)
		"'London', NULL, 'GB'",
		// NULL for zero FrequencyMHz on NTS (internet-only)
		"'internet', NULL, 'nts_api'",
		// NULL for empty HostName on "The NTS Breakfast Show" (rotating hosts)
		"'The NTS Breakfast Show', 'breakfast-show-nts', NULL,",
	}
	for _, want := range mustContain {
		if !strings.Contains(sql, want) {
			t.Errorf("generated SQL missing expected substring:\n  want: %q", want)
		}
	}

	// Exactly two ON CONFLICT clauses (one per table).
	if got := strings.Count(sql, "ON CONFLICT"); got != 2 {
		t.Errorf("want 2 ON CONFLICT clauses (stations + shows), got %d", got)
	}
}

func TestSqlString_EscapesApostrophes(t *testing.T) {
	cases := map[string]string{
		"hello":          "'hello'",
		"KEXP's flagship": "'KEXP''s flagship'",
		"don't":          "'don''t'",
		"o'b'r'ien":      "'o''b''r''ien'",
		"":               "''",
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
	// Cheap count: commas after the NOW(), NOW()) pattern + 2 non-comma
	// terminals (one per table).
	rows := strings.Count(sql, "NOW(), NOW()),") + 2
	want := len(RadioStations) + len(RadioShows)
	if rows != want {
		t.Errorf("row count mismatch: got %d, want %d (stations=%d + shows=%d)",
			rows, want, len(RadioStations), len(RadioShows))
	}
}
