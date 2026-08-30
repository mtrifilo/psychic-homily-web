package db

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// requiredColumn names a table column the running server depends on.
// Add entries here when a migration introduces columns that hot code paths
// assume exist — drift between schema_migrations and actual DDL then fails
// boot instead of surfacing as silent 422s at request time (PSY-1384).
type requiredColumn struct {
	Table  string
	Column string
}

// requiredSchemaColumns is the boot-time checklist. Keep it small: only
// columns whose absence breaks core request paths before any health probe
// would notice.
var requiredSchemaColumns = []requiredColumn{
	{Table: "user_bookmarks", Column: "scene_digest_sent_at"},
	{Table: "user_preferences", Column: "notify_on_scene_digest"},
	// PSY-1907: every follow's alert subscription resolves against this column,
	// so recorded-migration-but-absent-DDL fails the Library following page and
	// both follow-alert endpoints at once rather than degrading.
	{Table: "user_preferences", Column: "alert_defaults"},
	// GORM's Create builds an explicit column list from the model, so these
	// three appear in every show INSERT. Absent DDL breaks show submission at
	// request time rather than at boot, which is exactly what this list is for.
	{Table: "shows", Column: "doors_at"},
	{Table: "shows", Column: "music_at"},
	{Table: "shows", Column: "door_price"},
	// PSY-1761: shared.VenueTZJoin reads this table on EVERY show-listing
	// query, so recorded-migration-but-absent-DDL here does not degrade, it
	// fails every listing surface at once with "relation does not exist" —
	// a worse outage than the unknown-zone raise the guard exists to remove.
	// Checked by a generated column so the assertion also catches a table
	// that exists but predates the shape the guard queries.
	{Table: "timezone_names_snapshot", Column: "name_lower"},
}

type columnChecker interface {
	HasColumn(table interface{}, column string) bool
}

func assertRequiredSchema(checker columnChecker) error {
	var missing []string
	for _, col := range requiredSchemaColumns {
		if !checker.HasColumn(col.Table, col.Column) {
			missing = append(missing, col.Table+"."+col.Column)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"required schema columns missing: %s (migration recorded but DDL absent?); refusing to boot",
			strings.Join(missing, ", "),
		)
	}
	return nil
}

// AssertRequiredSchema verifies that critical columns exist before the server
// accepts traffic. Call after db.Connect in cmd/server.
func AssertRequiredSchema(gormDB *gorm.DB) error {
	if gormDB == nil {
		return fmt.Errorf("database connection is nil")
	}
	if err := assertRequiredSchema(gormDB.Migrator()); err != nil {
		return err
	}
	return assertTimezoneSnapshotPopulated(gormDB)
}

// assertTimezoneSnapshotPopulated refuses to boot against an EMPTY
// timezone_names_snapshot (PSY-1761).
//
// This is the only column in this file's remit whose emptiness matters as much
// as its absence, and it is checked here because it is the one failure mode
// that produces no error anywhere. shared.VenueTZJoin tests each venue's stored
// zone for membership in this table; against an empty one every test fails, so
// every venue silently falls back to the US state map and every non-US venue to
// UTC — up to fourteen hours off, enough to move a show to the wrong calendar
// day, with no exception raised and nothing logged. The drift detector cannot
// see it either: it compares venues against the LIVE catalog, which still
// agrees with them.
//
// An out-of-band TRUNCATE or a selective restore is what would produce it (a
// stage-to-production data refresh restores table data, and the refresher's own
// emptiness guard only covers the catalog read returning nothing). Failing the
// boot turns a silent, sitewide, wrong-date condition into an obvious one that
// `migrate up` on the next deploy repairs.
func assertTimezoneSnapshotPopulated(gormDB *gorm.DB) error {
	var zones int64
	if err := gormDB.Raw("SELECT count(*) FROM timezone_names_snapshot").Scan(&zones).Error; err != nil {
		return fmt.Errorf("could not read timezone_names_snapshot: %w; refusing to boot", err)
	}
	if zones == 0 {
		return fmt.Errorf(
			"timezone_names_snapshot is empty; every venue timezone would fail the venue-local " +
				"zone guard and every show would be dated by the state map instead. Re-seed it " +
				"(the create migration's INSERT ... SELECT FROM pg_timezone_names); refusing to boot")
	}
	return nil
}
