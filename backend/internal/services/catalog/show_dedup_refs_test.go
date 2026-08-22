package catalog

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// The drift guards for MergeDuplicateShow, plus the merge cases that prove the
// inventory is actually walked.
//
// Show dedup is destructive: it deletes a show row, and four of the five foreign
// keys to shows CASCADE. Every test here therefore runs against a real Postgres
// with the real constraints. Two of the cases below cannot be observed any other
// way, because both are constraint behaviour rather than Go behaviour:
// show_reports' UNIQUE (show_id, reported_by), and the self-referencing
// shows.duplicate_of_show_id.

// ──────────────────────────────────────────────
// Drift guards
// ──────────────────────────────────────────────

// TestShowEntityRefsCoverSchema is the guard PSY-1869 exists to add, and the
// third copy of an assertion the venue and artist suites already make against
// the same shared list.
//
// All three are kept deliberately: the list is shared, but each merge should
// fail on its own terms, so a reader of any one suite learns that merge is
// guarded. The failure mode being guarded is silent, which is what makes the
// repetition worth its cost: a new entity_type table simply keeps rows pointing
// at a deleted show, and nothing raises anything.
func (s *ShowDedupTestSuite) TestShowEntityRefsCoverSchema() {
	var tables []string
	s.Require().NoError(s.db.Raw(`
		SELECT table_name FROM information_schema.columns
		WHERE table_schema = 'public' AND column_name = 'entity_type'
		ORDER BY table_name
	`).Scan(&tables).Error)
	s.Require().NotEmpty(tables)

	unhandled := unhandledEntityRefTables(tables, entityRefTables())
	s.Emptyf(unhandled,
		"%v have an entity_type column but are in neither polymorphicEntityRefs nor "+
			"refsRepointedElsewhere. A show merge would leave their rows pointing at a deleted "+
			"show. Add each one (with its unique key), or handle it in a dedicated step and name "+
			"it in refsRepointedElsewhere.", unhandled)

	// And the reverse: nothing in either list may have been dropped by a
	// migration, which would make every merge fail at runtime rather than here.
	present := map[string]bool{}
	for _, t := range tables {
		present[t] = true
	}
	for _, ref := range polymorphicEntityRefs {
		s.Truef(present[ref.table],
			"polymorphicEntityRefs lists %q, which no longer has an entity_type column", ref.table)
	}
	for _, table := range refsRepointedElsewhere {
		s.Truef(present[table],
			"refsRepointedElsewhere lists %q, which no longer has an entity_type column", table)
	}
}

// TestShowForeignKeysAreAllHandled is the second drift guard, and the one that
// covers the columns the database itself would quietly empty.
//
// All but one of these CASCADE and the remaining one sets NULL, so a column this
// merge does not handle raises nothing when the losing show goes: its rows are
// silently destroyed, or silently lose the show they pointed at. (Deliberately
// not a count — the count drifted the first time a migration added a column.)
//
// Matched per COLUMN, not per table, matching TestArtistForeignKeysAreAllHandled:
// shows already carries a foreign key to itself, so a table-level assertion
// would stay green when a migration added a second show column to a table
// already in the list, and the merge would re-point only the first.
func (s *ShowDedupTestSuite) TestShowForeignKeysAreAllHandled() {
	var columns []string
	s.Require().NoError(s.db.Raw(`
		SELECT DISTINCT kcu.table_name || '.' || kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		 AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
		  ON tc.constraint_name = ccu.constraint_name
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND ccu.table_name = 'shows'
		  AND ccu.column_name = 'id'
		ORDER BY 1
	`).Scan(&columns).Error)
	s.Require().NotEmpty(columns)

	s.ElementsMatch(showFKColumns, columns,
		"the set of columns with a foreign key to shows has changed. A new one must be given "+
			"a disposition in showFKColumns, and a re-point in MergeDuplicateShow unless the "+
			"disposition says otherwise. Most of these CASCADE, so an unhandled column loses "+
			"its rows silently when the merged show is deleted.")
}

// TestShowIDColumnsWithoutForeignKeysAreAccountedFor is the third guard, and the
// only one that could see a bare show id: a column with no foreign key and no
// entity_type discriminator, invisible to both lists above.
//
// The expected list is EMPTY today, which is the reason to assert it rather than
// skip it. The artist merge found notification_filters.artist_ids sitting in
// exactly this shape, unenforced by anything, and the show equivalent would be
// just as invisible until someone read every migration.
//
// An empty expectation makes a broken query indistinguishable from a clean
// schema, so the same query is run WITHOUT the foreign-key exclusion first: that
// half must find columns, which proves the name pattern still matches something
// before the exclusion is trusted to have emptied it.
func (s *ShowDedupTestSuite) TestShowIDColumnsWithoutForeignKeysAreAccountedFor() {
	const namePattern = `
		  AND c.column_name LIKE '%show%'
		  AND (c.column_name LIKE '%id' OR c.column_name LIKE '%ids')`

	var allShowIDColumns []string
	s.Require().NoError(s.db.Raw(`
		SELECT c.table_name || '.' || c.column_name
		FROM information_schema.columns c
		WHERE c.table_schema = 'public'` + namePattern + `
		ORDER BY 1
	`).Scan(&allShowIDColumns).Error)
	s.Require().NotEmpty(allShowIDColumns,
		"the column-name pattern matches nothing at all, so the guard below cannot fail")

	var unconstrained []string
	s.Require().NoError(s.db.Raw(`
		SELECT c.table_name || '.' || c.column_name
		FROM information_schema.columns c
		WHERE c.table_schema = 'public'` + namePattern + `
		  AND NOT EXISTS (
		        SELECT 1
		        FROM information_schema.key_column_usage kcu
		        JOIN information_schema.table_constraints tc
		          ON tc.constraint_name = kcu.constraint_name
		         AND tc.constraint_type = 'FOREIGN KEY'
		        WHERE kcu.table_schema = c.table_schema
		          AND kcu.table_name = c.table_name
		          AND kcu.column_name = c.column_name
		      )
		ORDER BY 1
	`).Scan(&unconstrained).Error)

	s.ElementsMatch(showUnconstrainedIDColumns, unconstrained,
		"a column holding show ids with no foreign key has been added or removed. Neither the "+
			"entity_type guard nor the foreign-key guard can see one, so it must be given a "+
			"disposition in showUnconstrainedIDColumns, even if that disposition is 'a radio "+
			"show id, nothing to re-point'.")
}

// ──────────────────────────────────────────────
// The sweep: every inventoried table actually moves
// ──────────────────────────────────────────────

// showRefFixtures are the rows a seeded reference needs to satisfy its own
// foreign keys.
type showRefFixtures struct {
	user         *authm.User
	tagID        uint
	collectionID uint
}

// refsThatCannotHoldAShow are the inventory tables whose CHECK constraint
// forbids entity_type='show'. They are still walked by the merge (one exhaustive
// list beats a list plus per-entity exceptions), but they cannot be seeded here,
// and saying so is what keeps the sweep below honest about its reach.
var refsThatCannotHoldAShow = map[string]bool{
	"image_enrich_queue": true, // artist / release
	"source_configs":     true, // venue / label
}

// seedShowRefRow inserts one row in table pointing at showID.
//
// The default branch is the forcing function: a migration that adds a reference
// table makes TestMergeDuplicateShow_MovesEveryInventoriedReference fail here
// with the table's name, so "the merge handles it" cannot be claimed by adding a
// line to an inventory without proving a row survives the merge.
func (s *ShowDedupTestSuite) seedShowRefRow(table string, showID uint, f showRefFixtures) {
	exec := func(sql string, args ...any) {
		s.Require().NoErrorf(s.db.Exec(sql, args...).Error, "seed %s", table)
	}
	switch table {
	case "audit_logs":
		exec(`INSERT INTO audit_logs (action, entity_type, entity_id) VALUES ('update', 'show', ?)`, showID)
	case "collection_items":
		exec(`INSERT INTO collection_items (collection_id, entity_type, entity_id, position, added_by_user_id)
		      VALUES (?, 'show', ?, 0, ?)`, f.collectionID, showID, f.user.ID)
	case "comment_last_read":
		exec(`INSERT INTO comment_last_read (user_id, entity_type, entity_id) VALUES (?, 'show', ?)`,
			f.user.ID, showID)
	case "comment_subscriptions":
		exec(`INSERT INTO comment_subscriptions (user_id, entity_type, entity_id) VALUES (?, 'show', ?)`,
			f.user.ID, showID)
	case "comments":
		exec(`INSERT INTO comments (entity_type, entity_id, user_id, body, body_html)
		      VALUES ('show', ?, ?, 'nice bill', '<p>nice bill</p>')`, showID, f.user.ID)
	case "entity_edit_audit_logs":
		exec(`INSERT INTO entity_edit_audit_logs (entity_type, entity_id) VALUES ('show', ?)`, showID)
	case "entity_reports":
		exec(`INSERT INTO entity_reports (entity_type, entity_id, reported_by, report_type)
		      VALUES ('show', ?, ?, 'duplicate')`, showID, f.user.ID)
	case "entity_requests":
		exec(`INSERT INTO entity_requests (entity_type, payload, requester_id, created_entity_id)
		      VALUES ('show', '{}'::jsonb, ?, ?)`, f.user.ID, showID)
	case "entity_tags":
		exec(`INSERT INTO entity_tags (tag_id, entity_type, entity_id, added_by_user_id)
		      VALUES (?, 'show', ?, ?)`, f.tagID, showID, f.user.ID)
	case "notification_log":
		exec(`INSERT INTO notification_log (user_id, entity_type, entity_id, channel)
		      VALUES (?, 'show', ?, 'email')`, f.user.ID, showID)
	case "pending_entity_edits":
		s.seedShowPendingEdit(showID, f.user.ID)
	case "requests":
		exec(`INSERT INTO requests (title, entity_type, requester_id, requested_entity_id)
		      VALUES ('add the openers', 'show', ?, ?)`, f.user.ID, showID)
	case "revisions":
		seedRevision(s.T(), s.db, "show", showID, f.user.ID, "fixed the door time")
	case "tag_votes":
		exec(`INSERT INTO tag_votes (tag_id, entity_type, entity_id, user_id, vote)
		      VALUES (?, 'show', ?, ?, 1)`, f.tagID, showID, f.user.ID)
	case "user_bookmarks":
		exec(`INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action)
		      VALUES (?, 'show', ?, 'save')`, f.user.ID, showID)
	default:
		s.Failf("no seed for a new reference table",
			"%s is in the merge's inventory but this sweep does not know how to seed it. Add a "+
				"case that inserts a row pointing at a show, or record it in "+
				"refsThatCannotHoldAShow with the CHECK that forbids entity_type='show'.", table)
	}
}

// The sweep, and the strongest form of the coverage guard: seed a row in EVERY
// inventory table that can hold a show, merge, and assert per table that the row
// followed the show onto the winner.
//
// Unlike the artist merge's equivalent, this does not pass vacuously on tables
// it forgot to seed. Every table is either seeded or recorded as unable to hold
// a show, and a table that is neither fails in seedShowRefRow by name. That is
// what makes "the coverage guard extends to the show-dedup path" true for
// behaviour and not only for list membership: adding a table to the inventory
// without teaching this merge to move its rows cannot reach main.
func (s *ShowDedupTestSuite) TestMergeDuplicateShow_MovesEveryInventoriedReference() {
	a := s.seedArtist("Sweep")
	v := s.seedVenue("Sweep Hall", "Phoenix", "AZ")
	eventDate := time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC)
	winner := s.seedShow("SW", eventDate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	loser := s.seedShow("SL", eventDate, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")

	f := showRefFixtures{user: s.seedUser("sweep@test.com")}
	row := s.db.Raw(`INSERT INTO tags (name, slug) VALUES ('sweep', ?) RETURNING id`,
		fmt.Sprintf("sweep-%d", time.Now().UnixNano())).Row()
	s.Require().NoError(row.Scan(&f.tagID))
	row = s.db.Raw(`INSERT INTO collections (title, slug, creator_id) VALUES ('Sweep', ?, ?) RETURNING id`,
		fmt.Sprintf("sweep-%d", time.Now().UnixNano()), f.user.ID).Row()
	s.Require().NoError(row.Scan(&f.collectionID))

	seeded := map[string]bool{}
	for _, ref := range polymorphicEntityRefs {
		if refsThatCannotHoldAShow[ref.table] {
			s.assertCheckForbidsShow(ref.table)
			continue
		}
		s.seedShowRefRow(ref.table, loser, f)
		seeded[ref.table] = true
	}
	for _, table := range refsRepointedElsewhere {
		s.seedShowRefRow(table, loser, f)
		seeded[table] = true
	}

	summary := &ShowDedupSummary{}
	s.Require().NoError(s.db.Transaction(func(tx *gorm.DB) error {
		return MergeDuplicateShow(tx, winner, loser, summary)
	}))

	for _, ref := range polymorphicEntityRefs {
		s.assertShowRefMoved(ref.table, ref.idCol, winner, loser, seeded[ref.table])
		if seeded[ref.table] {
			s.Equalf(int64(1), summary.EntityRefsMoved[ref.table],
				"the CLI summary must report %s's moved row", ref.table)
		}
	}
	for _, table := range refsRepointedElsewhere {
		s.assertShowRefMoved(table, "entity_id", winner, loser, seeded[table])
	}

	// The history tables keep named counters rather than joining the map, because
	// each moves through its own provenance decision.
	s.Equal(int64(1), summary.PendingEditsMoved)
	s.Equal(int64(1), summary.EditAuditLogsMoved,
		"entity_edit_audit_logs was never re-pointed by a show merge before PSY-1869")
	s.Equal(int64(1), summary.RevisionsMoved)
}

// assertCheckForbidsShow re-earns the sweep's permission to skip a table.
//
// refsThatCannotHoldAShow is a claim about a CHECK constraint, and a migration
// that relaxed one would leave the sweep silently skipping a table that CAN now
// hold a show, which is the exact silent narrowing these guards exist to
// prevent. So the claim is verified against the live constraint rather than
// trusted.
func (s *ShowDedupTestSuite) assertCheckForbidsShow(table string) {
	var clauses []string
	s.Require().NoError(s.db.Raw(`
		SELECT pg_get_constraintdef(con.oid)
		FROM pg_constraint con
		JOIN pg_class rel ON rel.oid = con.conrelid
		JOIN pg_namespace n ON n.oid = rel.relnamespace
		WHERE n.nspname = 'public' AND con.contype = 'c' AND rel.relname = ?
		  AND pg_get_constraintdef(con.oid) LIKE '%entity_type%'
	`, table).Scan(&clauses).Error)
	s.Require().NotEmptyf(clauses,
		"%s is listed in refsThatCannotHoldAShow but has no CHECK on entity_type at all, "+
			"so the sweep is skipping a table it should be seeding", table)

	for _, clause := range clauses {
		s.NotContainsf(clause, "'show'",
			"%s's entity_type CHECK now admits 'show': %s. Remove it from "+
				"refsThatCannotHoldAShow and add a seed case, or the sweep skips a table the "+
				"merge really has to move.", table, clause)
	}
}

// assertShowRefMoved checks one table both ways: nothing is left pointing at the
// deleted show, and the row that was seeded is now on the winner.
func (s *ShowDedupTestSuite) assertShowRefMoved(table, idCol string, winner, loser uint, wasSeeded bool) {
	// #nosec G201 -- table and column come from the hardcoded inventory.
	countSQL := fmt.Sprintf(
		"SELECT COUNT(*) FROM %s WHERE entity_type = 'show' AND %s = ?", table, idCol)

	var stale int64
	s.Require().NoError(s.db.Raw(countSQL, loser).Scan(&stale).Error)
	s.Zerof(stale, "%s still points at the merged-away show", table)

	if !wasSeeded {
		return
	}
	var onWinner int64
	s.Require().NoError(s.db.Raw(countSQL, winner).Scan(&onWinner).Error)
	s.Equalf(int64(1), onWinner, "%s's row did not arrive on the surviving show", table)
}

// ──────────────────────────────────────────────
// Constraints the old hand-written re-points could not survive
// ──────────────────────────────────────────────

// show_reports is UNIQUE (show_id, reported_by) and moved with a bare UPDATE, so
// one user who reported BOTH duplicates aborted the entire cluster transaction
// with a unique violation.
//
// That reporter is not a corner case. A duplicate cluster is the same event
// listed twice, so a user who sees it cancelled or sees the time wrong is
// looking at two listings with the same defect, and the report types
// ('cancelled', 'sold_out', 'inaccurate') all describe the EVENT rather than the
// row. Filing on both is the diligent thing to do.
func (s *ShowDedupTestSuite) TestMergeDuplicateShow_SurvivesOneReporterOnBothShows() {
	a := s.seedArtist("Reported")
	v := s.seedVenue("Report Hall", "Phoenix", "AZ")
	eventDate := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	winner := s.seedShow("RW", eventDate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	loser := s.seedShow("RL", eventDate, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")

	bothShows := s.seedUser("diligent@test.com")
	loserOnly := s.seedUser("passerby@test.com")

	report := `INSERT INTO show_reports (show_id, reported_by, report_type, details) VALUES (?, ?, ?, ?)`
	s.Require().NoError(s.db.Exec(report, winner, bothShows.ID, "cancelled", "venue posted a cancellation").Error)
	s.Require().NoError(s.db.Exec(report, loser, bothShows.ID, "cancelled", "same cancellation, other listing").Error)
	s.Require().NoError(s.db.Exec(report, loser, loserOnly.ID, "inaccurate", "wrong door time").Error)

	summary := &ShowDedupSummary{}
	s.Require().NoError(s.db.Transaction(func(tx *gorm.DB) error {
		return MergeDuplicateShow(tx, winner, loser, summary)
	}))

	var byBoth, byPasserby int64
	s.Require().NoError(s.db.Raw(
		`SELECT COUNT(*) FROM show_reports WHERE show_id = ? AND reported_by = ?`,
		winner, bothShows.ID).Scan(&byBoth).Error)
	s.Equal(int64(1), byBoth,
		"the duplicate report must be dropped rather than re-pointed into a unique violation")

	s.Require().NoError(s.db.Raw(
		`SELECT COUNT(*) FROM show_reports WHERE show_id = ? AND reported_by = ?`,
		winner, loserOnly.ID).Scan(&byPasserby).Error)
	s.Equal(int64(1), byPasserby, "a report with no counterpart on the winner must survive")

	s.Equal(int64(1), summary.ShowReportsMoved)
	s.Equal(int64(1), summary.ShowReportsSkipped,
		"a dropped report must be visible in the CLI summary, not silent")
}

// shows.duplicate_of_show_id points at shows, so the winner's own row is in the
// set the re-point sweeps. Flagging the loser as a duplicate of the winner is
// exactly what an admin does BEFORE running dedup, which makes this the ordinary
// path rather than an exotic one, and the bare UPDATE turned it into a show
// recorded as a duplicate of itself.
//
// The column is served to clients on the show payload, so a self-reference is
// not merely untidy: it is a live claim that a show duplicates itself.
func (s *ShowDedupTestSuite) TestMergeDuplicateShow_DoesNotFlagTheWinnerAsItsOwnDuplicate() {
	a := s.seedArtist("Flagged")
	v := s.seedVenue("Flagged Hall", "Phoenix", "AZ")
	eventDate := time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)
	winner := s.seedShow("FW", eventDate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	loser := s.seedShow("FL", eventDate, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	other := s.seedShow("FO", eventDate.AddDate(0, 0, 1),
		time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")

	// The winner was already marked as a possible duplicate of the loser, and a
	// third show points at the loser too.
	s.Require().NoError(s.db.Exec(
		`UPDATE shows SET duplicate_of_show_id = ? WHERE id IN ?`,
		loser, []uint{winner, other}).Error)

	summary := &ShowDedupSummary{}
	s.Require().NoError(s.db.Transaction(func(tx *gorm.DB) error {
		return MergeDuplicateShow(tx, winner, loser, summary)
	}))

	// The winner's own flag is left pointing at the loser, so deleting the loser
	// clears it through the column's ON DELETE SET NULL. That is the honest
	// answer: the show it was flagged against no longer exists. Re-pointing it
	// would have written the winner's own id there instead.
	var survivor catalogm.Show
	s.Require().NoError(s.db.First(&survivor, winner).Error)
	if survivor.DuplicateOfShowID != nil {
		s.NotEqual(winner, *survivor.DuplicateOfShowID,
			"a show must never be recorded as a duplicate of itself")
	}
	s.Nil(survivor.DuplicateOfShowID,
		"the flag against the merged-away show should be cleared by its delete, not re-pointed "+
			"at the surviving show itself")

	var third catalogm.Show
	s.Require().NoError(s.db.First(&third, other).Error)
	s.Require().NotNil(third.DuplicateOfShowID)
	s.Equal(winner, *third.DuplicateOfShowID,
		"another show's flag must follow the merged-away show onto the winner")
	s.Equal(int64(1), summary.DuplicateOfRepoint)
}

// ──────────────────────────────────────────────
// The summary's own drift guard (no database)
// ──────────────────────────────────────────────

// ShowDedupSummary.Add is a hand-written field-by-field fold, which is the same
// shape of hand-maintained list this ticket removed from the merge. A field
// added to the struct and forgotten here would silently report zero for work
// that really happened, and no other test would notice.
//
// So the fold is checked generically: fill every field of two summaries with
// distinct values, fold, and require every field to hold the sum. A new field is
// covered the moment it is declared.
func TestShowDedupSummaryAddFoldsEveryField(t *testing.T) {
	const (
		left  = 3
		right = 5
		key   = "some_table"
	)

	fill := func(s *ShowDedupSummary, n int64) {
		v := reflect.ValueOf(s).Elem()
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			switch f.Kind() {
			case reflect.Int, reflect.Int64:
				f.SetInt(n)
			case reflect.Map:
				f.Set(reflect.ValueOf(map[string]int64{key: n}))
			default:
				t.Fatalf("field %s has kind %s, which this guard does not know how to "+
					"fill or check; teach it, do not skip it",
					v.Type().Field(i).Name, f.Kind())
			}
		}
	}

	total := &ShowDedupSummary{}
	cluster := &ShowDedupSummary{}
	fill(total, left)
	fill(cluster, right)

	total.Add(cluster)

	v := reflect.ValueOf(total).Elem()
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		f := v.Field(i)
		switch f.Kind() {
		case reflect.Int, reflect.Int64:
			if f.Int() != left+right {
				t.Errorf("%s = %d after Add, want %d. ShowDedupSummary.Add does not fold it, "+
					"so a dedup pass would under-report it.", name, f.Int(), left+right)
			}
		case reflect.Map:
			got := f.Interface().(map[string]int64)
			if got[key] != left+right {
				t.Errorf("%s[%q] = %d after Add, want %d", name, key, got[key], left+right)
			}
		}
	}

	// The cluster summary must not be mutated by being folded in: the caller
	// prints or discards it afterwards.
	if cluster.LosersMerged != right {
		t.Errorf("Add mutated the folded-in summary: LosersMerged = %d, want %d",
			cluster.LosersMerged, right)
	}
}

// A nil cluster summary is what a caller passes when nothing ran, and it must
// not panic in the middle of a CLI pass that has already committed work.
func TestShowDedupSummaryAddIgnoresNil(t *testing.T) {
	total := &ShowDedupSummary{LosersMerged: 2}
	total.Add(nil)
	if total.LosersMerged != 2 {
		t.Errorf("LosersMerged = %d, want 2", total.LosersMerged)
	}
}

// A merge whose loser is already gone used to return success: every re-point
// matched zero rows, GORM reports no error for a delete that matches nothing,
// and LosersMerged was incremented anyway. The CLI then printed a merge that
// never happened, in the one report anyone has of a destructive pass.
func (s *ShowDedupTestSuite) TestMergeDuplicateShow_RejectsAVanishedLoser() {
	a := s.seedArtist("Vanished")
	v := s.seedVenue("Vanished Hall", "Phoenix", "AZ")
	eventDate := time.Date(2026, 12, 1, 3, 0, 0, 0, time.UTC)
	winner := s.seedShow("VW", eventDate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	loser := s.seedShow("VL", eventDate, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")

	s.Require().NoError(s.db.Exec(`DELETE FROM show_artists WHERE show_id = ?`, loser).Error)
	s.Require().NoError(s.db.Exec(`DELETE FROM show_venues WHERE show_id = ?`, loser).Error)
	s.Require().NoError(s.db.Exec(`DELETE FROM shows WHERE id = ?`, loser).Error)

	summary := &ShowDedupSummary{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		return MergeDuplicateShow(tx, winner, loser, summary)
	})
	s.Require().Error(err, "merging a show that no longer exists must fail, not report success")
	s.Contains(err.Error(), "no longer exists")
	s.Zero(summary.LosersMerged, "a merge that did not happen must not be counted")
}

// ──────────────────────────────────────────────
// moveShowFKRows' own guards (no database)
// ──────────────────────────────────────────────

// The re-point is refused for a column with no recorded disposition, which is
// what makes showFKColumns load-bearing rather than documentary. It is also the
// SQL-injection fence: the table and column are interpolated into the statement,
// and this pins them to a package-level list rather than to "every call site
// happens to pass a literal today".
func TestMoveShowFKRowsRejectsAnUndispositionedColumn(t *testing.T) {
	for _, tc := range []struct {
		name           string
		table, showCol string
		dedupeOn       string
	}{
		{"unlisted table", "shows_backup", "show_id", "venue_id"},
		{"listed table, wrong column", "show_venues", "id", "venue_id"},
		{"injection attempt", "show_venues; DROP TABLE shows --", "show_id", "venue_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := moveShowFKRows(unusableTx(), tc.table, tc.showCol, tc.dedupeOn, 1, 2)
			if err == nil {
				t.Fatalf("%s.%s must be refused: it has no disposition in showFKColumns",
					tc.table, tc.showCol)
			}
		})
	}

	// A column whose recorded disposition is "drop" must be refused even though the
	// completeness inventory lists it. Listing it there is mandatory (the FK guard
	// asserts the inventory equals the schema), so without this second fence the
	// listing itself would grant permission to do the one thing that column's
	// disposition forbids.
	for _, table := range showFKColumnsNeverRepointed {
		if !showFKColumnListed(table) {
			t.Errorf("%s is drop-only but missing from showFKColumns, which the FK guard requires", table)
		}
	}
	if _, _, err := moveShowFKRows(unusableTx(), "show_notify_queue", "show_id", "id", 1, 2); err == nil {
		t.Error("show_notify_queue.show_id is drop-only: re-pointing it would either abort the " +
			"merge on UNIQUE (show_id) or let a merge announce a show that predates the feature")
	}

	// And the fence must not be so tight that the real call sites cannot pass it.
	// Checked against the predicate rather than through moveShowFKRows, because
	// an accepted column goes on to execute and a zero *gorm.DB panics there.
	for _, table := range []string{"show_venues", "show_artists", "show_reports"} {
		if !showFKColumnListed(table + ".show_id") {
			t.Errorf("%s.show_id is a real call site in MergeDuplicateShow and must be listed "+
				"in showFKColumns, or every merge fails", table)
		}
	}
}

// The self-merge and zero-id rejections matter for the same reason they do on
// the shared helpers: the delete correlates rows against their own table, so a
// self-merge would drop the surviving show's rows before a no-op move.
func TestMoveShowFKRowsRejectsSelfMergeAndZeroIDs(t *testing.T) {
	if _, _, err := moveShowFKRows(unusableTx(), "show_venues", "show_id", "venue_id", 7, 7); err == nil {
		t.Error("a self-merge would delete the surviving show's own rows")
	}
	if _, _, err := moveShowFKRows(unusableTx(), "show_venues", "show_id", "venue_id", 0, 2); err == nil {
		t.Error("a zero winner id must be rejected")
	}
	if _, _, err := moveShowFKRows(unusableTx(), "show_venues", "show_id", "venue_id", 1, 0); err == nil {
		t.Error("a zero loser id must be rejected")
	}
}
