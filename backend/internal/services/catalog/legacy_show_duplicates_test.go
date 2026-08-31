package catalog

import (
	"fmt"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedLegacyShowDuplicate runs fn with the show_dedup_keys triggers switched
// off, then derives the keys the migration's backfill would have left behind.
//
// The trigger disable is the point rather than a workaround. show_dedup_keys
// forbids two shows billing one artist at one venue on one instant, so every
// such pair still in the catalog is data that PREDATES the constraint, and
// collapsing exactly those pairs is what cmd/dedup-shows and the merges exist to
// do. A fixture that could not build one could not test them.
//
// The backfill replay is what makes the fixture FAITHFUL, and leaving it out
// under-tested the merges rather than merely simplifying them. Post-migration
// legacy data is not "a duplicate pair with no keys": the backfill takes every
// billing it can and skips only the ones that actually collide, so a losing show
// still holds a live key for every act the winner does not bill. Those are the
// keys a merge has to move without colliding. Seeding with none of them made the
// merge's ordering requirement unobservable — reversing the trigger's OLD/NEW
// rebuild order left the whole suite green.
//
// Replayed as the migration writes it, ON CONFLICT DO NOTHING over the whole
// catalog, so the fixture cannot drift into deriving a different set.
//
// Only the SEEDING runs unguarded. The triggers are back on before the code
// under test runs, so the merge is still exercised against the live constraint.
func seedLegacyShowDuplicate(t require.TestingT, db *gorm.DB, fn func()) {
	triggers := showDedupKeyTriggers(t, db)
	setShowDedupKeyTriggers(t, db, triggers, false)
	fn()
	setShowDedupKeyTriggers(t, db, triggers, true)
	backfillShowDedupKeys(t, db)
}

// backfillShowDedupKeys mirrors the 20260830224500 migration's backfill.
func backfillShowDedupKeys(t require.TestingT, db *gorm.DB) {
	require.NoError(t, db.Exec(`
		INSERT INTO show_dedup_keys (show_id, artist_id, venue_id, event_date)
		SELECT sa.show_id, sa.artist_id, sv.venue_id, s.event_date
		FROM show_artists sa
		JOIN show_venues sv ON sv.show_id = sa.show_id
		JOIN shows s        ON s.id       = sa.show_id
		ORDER BY s.created_at, s.id
		ON CONFLICT DO NOTHING
	`).Error)
}

// dedupKeyTrigger is one trigger to switch, named by table.
type dedupKeyTrigger struct {
	Table string `gorm:"column:table_name"`
	Name  string `gorm:"column:trigger_name"`
}

// showDedupKeyTriggers reads the set from the catalog rather than keeping a copy
// of it here. A migration that adds a fourth trigger would leave a hand-written
// list half-disabling the derivation, and a half-derived table is a worse state
// than either a derived or an underived one.
func showDedupKeyTriggers(t require.TestingT, db *gorm.DB) []dedupKeyTrigger {
	var triggers []dedupKeyTrigger
	require.NoError(t, db.Raw(`
		SELECT c.relname AS table_name, tg.tgname AS trigger_name
		FROM pg_trigger tg
		JOIN pg_class c ON c.oid = tg.tgrelid
		WHERE NOT tg.tgisinternal
		  AND tg.tgname LIKE '%\_sync\_dedup\_keys'
		ORDER BY 1, 2
	`).Scan(&triggers).Error)
	require.NotEmpty(t, triggers,
		"no show_dedup_keys triggers found; the naming this fixture matches on has changed")
	return triggers
}

func setShowDedupKeyTriggers(t require.TestingT, db *gorm.DB, triggers []dedupKeyTrigger, enabled bool) {
	verb := "DISABLE"
	if enabled {
		verb = "ENABLE"
	}
	for _, trigger := range triggers {
		require.NoError(t, db.Exec(
			fmt.Sprintf("ALTER TABLE %s %s TRIGGER %s", trigger.Table, verb, trigger.Name)).Error)
	}
}
