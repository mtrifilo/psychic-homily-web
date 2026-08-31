package catalog

import (
	"fmt"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// showDedupKeyTriggers are the triggers that derive show_dedup_keys, named so a
// fixture can switch them off. Kept in one place because switching two of three
// off leaves a half-derived table, which is worse than either state.
var showDedupKeyTriggers = map[string]string{
	"show_artists": "show_artists_sync_dedup_keys",
	"show_venues":  "show_venues_sync_dedup_keys",
	"shows":        "shows_sync_dedup_keys",
}

// seedLegacyShowDuplicate runs fn with those triggers off, which is the only way
// left to build a duplicate billing.
//
// That is the point rather than a workaround. show_dedup_keys forbids two shows
// billing one artist at one venue on one instant, so every such pair still in
// the catalog is data that PREDATES the constraint, and collapsing exactly those
// pairs is what cmd/dedup-shows and the merges exist to do. A fixture that could
// not build one could not test them. Switching the derivation off reproduces the
// history faithfully: the association rows exist and no key row was ever derived
// for the later show.
//
// Only the SEEDING runs unguarded. The triggers are back on before the code
// under test runs, so the merge is still exercised against the live constraint.
func seedLegacyShowDuplicate(t require.TestingT, db *gorm.DB, fn func()) {
	setShowDedupKeyTriggers(t, db, false)
	defer setShowDedupKeyTriggers(t, db, true)
	fn()
}

func setShowDedupKeyTriggers(t require.TestingT, db *gorm.DB, enabled bool) {
	verb := "DISABLE"
	if enabled {
		verb = "ENABLE"
	}
	for table, trigger := range showDedupKeyTriggers {
		require.NoError(t, db.Exec(
			fmt.Sprintf("ALTER TABLE %s %s TRIGGER %s", table, verb, trigger)).Error)
	}
}
