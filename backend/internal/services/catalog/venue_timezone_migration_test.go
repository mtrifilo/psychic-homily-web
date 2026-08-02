package catalog

import (
	"database/sql"
	"os"
	"testing"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// The PSY-1707 migration, exercised against real rows rather than reasoned
// about. It is expected to be a no-op in both deployed environments (measured:
// zero invalid or non-canonical values on stage and production), which is
// exactly why it needs a test -- a migration that changes nothing where you can
// see it, and everything where you cannot, is not one to ship unverified.
//
// Reads the .sql file itself so the assertions cannot drift from the statements
// that will actually run.
func TestNormalizeVenueTimezonesMigration(t *testing.T) {
	db := testutil.SetupTestPostgres(t).DB

	seed := []struct{ name, tz string }{
		{"canonical", "America/Phoenix"},
		{"lowercased", "america/phoenix"},
		{"padded", "  America/Phoenix  "},
		{"blank", ""},
		{"whitespace", "   "},
		{"junk", "Not/AZone"},
		{"go-only-abbrev", "EST"},
		{"go-only-alias", "Local"},
	}
	ids := map[string]uint{}
	for _, s := range seed {
		tz := s.tz
		v := &catalogm.Venue{Name: "Mig " + s.name, City: "Testville", State: "AZ", Timezone: &tz}
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
		ids[s.name] = v.ID
	}
	// A NULL row must be left alone -- NULL is a legitimate state (geocode miss),
	// not something to "fix".
	nullV := &catalogm.Venue{Name: "Mig null", City: "Testville", State: "AZ"}
	if err := db.Create(nullV).Error; err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile("../../../db/migrations/20260802043206_normalize_venue_timezones.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(body)).Error; err != nil {
		t.Fatalf("migration failed to execute: %v", err)
	}

	want := map[string]*string{
		"canonical":      ptr("America/Phoenix"),
		"lowercased":     ptr("America/Phoenix"),
		"padded":         ptr("America/Phoenix"),
		"blank":          nil,
		"whitespace":     nil,
		"junk":           nil,
		"go-only-abbrev": nil,
		"go-only-alias":  nil,
	}
	for name, id := range ids {
		var got sql.NullString
		if err := db.Raw("SELECT timezone FROM venues WHERE id = ?", id).Scan(&got).Error; err != nil {
			t.Fatal(err)
		}
		exp := want[name]
		switch {
		case exp == nil && got.Valid:
			t.Errorf("%s: expected NULL, got %q", name, got.String)
		case exp != nil && !got.Valid:
			t.Errorf("%s: expected %q, got NULL", name, *exp)
		case exp != nil && got.Valid && got.String != *exp:
			t.Errorf("%s: expected %q, got %q", name, *exp, got.String)
		default:
			t.Logf("%s -> %v OK", name, got)
		}
	}

	// Re-runnable: a migration that only works on a virgin table is a trap for
	// anyone replaying it against a partially-migrated environment.
	if err := db.Exec(string(body)).Error; err != nil {
		t.Fatalf("migration is not re-runnable: %v", err)
	}

	// Every surviving value must be usable by AT TIME ZONE.
	var bad int64
	if err := db.Raw(`SELECT count(*) FROM venues v WHERE v.timezone IS NOT NULL
		AND NOT EXISTS (SELECT 1 FROM pg_timezone_names t WHERE t.name = v.timezone)`).Scan(&bad).Error; err != nil {
		t.Fatal(err)
	}
	if bad != 0 {
		t.Errorf("%d venues still hold a zone Postgres cannot resolve", bad)
	}
}
