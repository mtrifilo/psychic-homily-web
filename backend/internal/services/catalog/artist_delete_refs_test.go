package catalog

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/testutil"
)

// The delete sweep's behavioural guards (PSY-1868).
//
// Every test here runs against a real Postgres, because the whole subject is
// what the DATABASE does with a row when the entity it names goes away, and the
// answer is "nothing at all" — none of these tables carries a foreign key, so
// none of them cascades and none of them raises. A unit test against a mock
// would agree with any implementation, correct or not.
//
// The centrepiece is TestDeleteArtist_HandlesEveryInventoriedReference, which
// SEEDS every inventory table before deleting and asserts each one's recorded
// disposition individually. List membership is not enough: a table can sit in
// the inventory, pass every coverage guard, and still have no handling behind
// it. Seeding is what makes the guard fail for the right reason.

type ArtistDeleteRefsSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *ArtistService
}

func (s *ArtistDeleteRefsSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = &ArtistService{db: s.db}
}

func (s *ArtistDeleteRefsSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *ArtistDeleteRefsSuite) SetupTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	// Ordered so that a table's dependants go first: pending_entity_edits and
	// revisions reference users with no ON DELETE clause, so a stale row makes
	// the users delete fail rather than the test that left it behind.
	for _, stmt := range []string{
		"DELETE FROM audit_logs",
		"DELETE FROM pending_entity_edits",
		"DELETE FROM entity_edit_audit_logs",
		"DELETE FROM revisions",
		"DELETE FROM comment_last_read",
		"DELETE FROM comment_subscriptions",
		"DELETE FROM comments",
		"DELETE FROM collection_items",
		"DELETE FROM collections",
		"DELETE FROM entity_reports",
		"DELETE FROM entity_requests",
		"DELETE FROM request_votes",
		"DELETE FROM requests",
		"DELETE FROM image_enrich_queue",
		"DELETE FROM notification_log",
		"DELETE FROM notification_filters",
		"DELETE FROM source_configs",
		"DELETE FROM tag_votes",
		"DELETE FROM entity_tags",
		"DELETE FROM tags",
		"DELETE FROM user_bookmarks",
		// artist_relationship_votes carries a composite FK to artist_relationships,
		// and artist_relationships is the ONE foreign key to artists with no
		// ON DELETE clause. A row either table keeps makes the artists delete below
		// fail rather than the test that left it behind, which is exactly what
		// TestDeleteArtist_RollsBackTheSweepWhenTheDeleteIsRefused seeds on purpose.
		"DELETE FROM artist_relationship_votes",
		"DELETE FROM artist_relationships",
		"DELETE FROM show_artists",
		"DELETE FROM show_venues",
		"DELETE FROM shows",
		"DELETE FROM artists",
		"DELETE FROM venues",
		"DELETE FROM users",
	} {
		_, err := sqlDB.Exec(stmt)
		s.Require().NoError(err, stmt)
	}
}

func TestArtistDeleteRefs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ArtistDeleteRefsSuite))
}

// ──────────────────────────────────────────────
// Fixtures
// ──────────────────────────────────────────────

// artistRefFixtures are the rows a seeded reference needs to satisfy its own
// foreign keys.
type artistRefFixtures struct {
	user         *authm.User
	tagID        uint
	collectionID uint
}

func (s *ArtistDeleteRefsSuite) seedArtist(name string) *catalogm.Artist {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	a := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a
}

func (s *ArtistDeleteRefsSuite) seedUser(email string) *authm.User {
	unique := fmt.Sprintf("%d-%s", time.Now().UnixNano(), email)
	u := &authm.User{Email: &unique, IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(u).Error)
	return u
}

func (s *ArtistDeleteRefsSuite) seedFixtures() artistRefFixtures {
	f := artistRefFixtures{user: s.seedUser("sweep@test.com")}
	unique := time.Now().UnixNano()

	// tags carries idx_tags_name_lower, UNIQUE on lower(name), so the NAME has to
	// be unique per call and not only the slug: this helper runs once per subtest.
	row := s.db.Raw(`INSERT INTO tags (name, slug) VALUES (?, ?) RETURNING id`,
		fmt.Sprintf("sweep-%d", unique), fmt.Sprintf("sweep-%d", unique)).Row()
	s.Require().NoError(row.Scan(&f.tagID))

	row = s.db.Raw(`INSERT INTO collections (title, slug, creator_id) VALUES ('Sweep', ?, ?) RETURNING id`,
		fmt.Sprintf("sweep-%d", unique), f.user.ID).Row()
	s.Require().NoError(row.Scan(&f.collectionID))

	return f
}

// refsThatCannotHoldAnArtist are the inventory tables whose CHECK constraint
// forbids entity_type='artist'. They are still walked by the sweep (one
// exhaustive list beats a list plus per-entity exceptions), but they cannot be
// seeded here, and saying so is what keeps the sweep honest about its reach.
var refsThatCannotHoldAnArtist = map[string]bool{
	"source_configs": true, // venue / label
}

// seedArtistRefRow inserts one row in table pointing at artistID.
//
// The default branch is the forcing function this ticket exists to add. A
// migration that adds a reference table makes the sweep below fail HERE, by
// name, so "the delete handles it" cannot be claimed by adding a line to the
// disposition map without proving the row is actually acted on.
func (s *ArtistDeleteRefsSuite) seedArtistRefRow(table string, artistID uint, f artistRefFixtures) {
	exec := func(sql string, args ...any) {
		s.Require().NoErrorf(s.db.Exec(sql, args...).Error, "seed %s", table)
	}
	switch table {
	case "audit_logs":
		exec(`INSERT INTO audit_logs (action, entity_type, entity_id) VALUES ('update', 'artist', ?)`,
			artistID)
	case "collection_items":
		exec(`INSERT INTO collection_items (collection_id, entity_type, entity_id, position, added_by_user_id)
		      VALUES (?, 'artist', ?, 0, ?)`, f.collectionID, artistID, f.user.ID)
	case "comment_last_read":
		exec(`INSERT INTO comment_last_read (user_id, entity_type, entity_id) VALUES (?, 'artist', ?)`,
			f.user.ID, artistID)
	case "comment_subscriptions":
		exec(`INSERT INTO comment_subscriptions (user_id, entity_type, entity_id) VALUES (?, 'artist', ?)`,
			f.user.ID, artistID)
	case "comments":
		exec(`INSERT INTO comments (entity_type, entity_id, user_id, body, body_html)
		      VALUES ('artist', ?, ?, 'great band', '<p>great band</p>')`, artistID, f.user.ID)
	case "entity_edit_audit_logs":
		exec(`INSERT INTO entity_edit_audit_logs (entity_type, entity_id) VALUES ('artist', ?)`, artistID)
	case "entity_reports":
		exec(`INSERT INTO entity_reports (entity_type, entity_id, reported_by, report_type)
		      VALUES ('artist', ?, ?, 'duplicate')`, artistID, f.user.ID)
	case "entity_requests":
		exec(`INSERT INTO entity_requests (entity_type, payload, requester_id, created_entity_id)
		      VALUES ('artist', '{}'::jsonb, ?, ?)`, f.user.ID, artistID)
	case "entity_tags":
		exec(`INSERT INTO entity_tags (tag_id, entity_type, entity_id, added_by_user_id)
		      VALUES (?, 'artist', ?, ?)`, f.tagID, artistID, f.user.ID)
		// The counter AddTagToEntity would have incremented. Seeded by hand so the
		// sweep's decrement has something true to give back.
		exec(`UPDATE tags SET usage_count = usage_count + 1 WHERE id = ?`, f.tagID)
	case "image_enrich_queue":
		exec(`INSERT INTO image_enrich_queue (entity_type, entity_id) VALUES ('artist', ?)`, artistID)
	case "notification_log":
		exec(`INSERT INTO notification_log (user_id, entity_type, entity_id, channel)
		      VALUES (?, 'artist', ?, 'email')`, f.user.ID, artistID)
	case "pending_entity_edits":
		s.seedArtistPendingEdit(artistID, f.user.ID)
	case "requests":
		exec(`INSERT INTO requests (title, entity_type, requester_id, requested_entity_id)
		      VALUES ('add this band', 'artist', ?, ?)`, f.user.ID, artistID)
	case "revisions":
		seedRevision(s.T(), s.db, "artist", artistID, f.user.ID, "fixed the hometown")
	case "tag_votes":
		exec(`INSERT INTO tag_votes (tag_id, entity_type, entity_id, user_id, vote)
		      VALUES (?, 'artist', ?, ?, 1)`, f.tagID, artistID, f.user.ID)
	case "user_bookmarks":
		exec(`INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action)
		      VALUES (?, 'artist', ?, 'follow')`, f.user.ID, artistID)
	default:
		s.Failf("no seed for a new reference table",
			"%s is in the entity-ref inventory but this sweep does not know how to seed it. Add "+
				"a case that inserts a row pointing at an artist, or record it in "+
				"refsThatCannotHoldAnArtist with the CHECK that forbids entity_type='artist'.", table)
	}
}

// seedArtistPendingEdit records one proposed artist edit. Written through the
// model so a column rename breaks the build rather than the test run.
func (s *ArtistDeleteRefsSuite) seedArtistPendingEdit(artistID, userID uint) {
	changes := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	edit := &adminm.PendingEntityEdit{
		EntityType:   adminm.PendingEditEntityArtist,
		EntityID:     artistID,
		SubmittedBy:  userID,
		FieldChanges: &changes,
		Summary:      "name fix",
		Status:       adminm.PendingEditStatusPending,
	}
	s.Require().NoError(s.db.Create(edit).Error)
}

// ──────────────────────────────────────────────
// The sweep
// ──────────────────────────────────────────────

// Seed a row in EVERY inventory table that can hold an artist, delete the
// artist, and assert per table that its RECORDED disposition is what actually
// happened.
//
// This is the guard the ticket asks for, in the strongest form PSY-1869
// established for the show merge: it cannot pass vacuously on a table it forgot
// to seed. Every table is either seeded or recorded as unable to hold an artist,
// and a table that is neither fails in seedArtistRefRow by name.
//
// The per-disposition assertions are what makes it more than a coverage check.
// "The row is gone" and "the row is still there on purpose" are both passes, and
// which one is correct is decided by entityRefDeleteDispositions rather than by
// this test — so flipping a disposition without meaning to fails here.
func (s *ArtistDeleteRefsSuite) TestDeleteArtist_HandlesEveryInventoriedReference() {
	artist := s.seedArtist("Sweep")
	f := s.seedFixtures()

	seeded := map[string]bool{}
	for table := range entityRefTables() {
		if refsThatCannotHoldAnArtist[table] {
			s.assertCheckForbidsArtist(table)
			continue
		}
		s.seedArtistRefRow(table, artist.ID, f)
		seeded[table] = true
	}

	// Every table that can hold an artist must actually have been seeded, or an
	// assertion below passes on an empty table and proves nothing.
	for _, ref := range polymorphicEntityRefs {
		if !refsThatCannotHoldAnArtist[ref.table] {
			s.Truef(seeded[ref.table], "%s was never seeded", ref.table)
			s.Equalf(int64(1), s.countRefRows(ref.table, ref.idCol, artist.ID),
				"%s's seed did not land", ref.table)
		}
	}

	s.Require().NoError(s.svc.DeleteArtist(artist.ID))

	for _, ref := range polymorphicEntityRefs {
		if !seeded[ref.table] {
			continue
		}
		s.assertDisposition(ref.table, ref.idCol, artist.ID)
	}
	for _, table := range refsRepointedElsewhere {
		s.assertDisposition(table, refsRepointedElsewhereIDCol, artist.ID)
	}

	// And the artist really is gone, so none of the above passed because the
	// delete silently failed.
	var remaining int64
	s.Require().NoError(s.db.Raw(`SELECT COUNT(*) FROM artists WHERE id = ?`, artist.ID).
		Scan(&remaining).Error)
	s.Zero(remaining, "the artist row must be deleted")
}

// assertDisposition checks one table against what entityRefDeleteDispositions
// says should have happened to it.
func (s *ArtistDeleteRefsSuite) assertDisposition(table, idCol string, artistID uint) {
	disposition, ok := entityRefDeleteDispositions[table]
	s.Require().Truef(ok, "%s has no recorded disposition", table)

	switch disposition {
	case dropRefRows:
		s.Zerof(s.countRefRows(table, idCol, artistID),
			"%s is dispositioned dropRefRows but its row survived the delete, pointing at an "+
				"artist id that no longer exists", table)

	case keepRefRowsAsTombstone:
		s.Equalf(int64(1), s.countRefRows(table, idCol, artistID),
			"%s is dispositioned keepRefRowsAsTombstone and its row must survive: it is read by "+
				"actor (contributor stats, trust tiers, the admin trail), which the deleted "+
				"artist does not gate", table)

	default:
		s.Failf("undecided disposition", "%s is recorded as %s", table, disposition)
	}
}

func (s *ArtistDeleteRefsSuite) countRefRows(table, idCol string, artistID uint) int64 {
	// #nosec G201 -- table and column come from the hardcoded inventory.
	sql := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE entity_type = 'artist' AND %s = ?", table, idCol)
	var n int64
	s.Require().NoError(s.db.Raw(sql, artistID).Scan(&n).Error)
	return n
}

// assertCheckForbidsArtist re-earns the sweep's permission to skip a table.
//
// refsThatCannotHoldAnArtist is a claim about a CHECK constraint, and a
// migration that relaxed one would leave the sweep silently skipping a table
// that CAN now hold an artist — the exact silent narrowing these guards exist to
// prevent. So the claim is verified against the live constraint rather than
// trusted.
func (s *ArtistDeleteRefsSuite) assertCheckForbidsArtist(table string) {
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
		"%s is listed in refsThatCannotHoldAnArtist but has no CHECK on entity_type at all, "+
			"so the sweep is skipping a table it should be seeding", table)

	for _, clause := range clauses {
		s.NotContainsf(clause, "'artist'",
			"%s's entity_type CHECK now admits 'artist': %s. Remove it from "+
				"refsThatCannotHoldAnArtist and add a seed case, or the sweep skips a table the "+
				"delete really has to handle.", table, clause)
	}
}

// ──────────────────────────────────────────────
// The parts a generic DELETE gets wrong
// ──────────────────────────────────────────────

// tags.usage_count is denormalized and hand-maintained — AddTagToEntity
// increments it, RemoveTagFromEntity decrements it, and nothing recomputes it —
// so the sweep's bare `DELETE FROM entity_tags` would leave it permanently
// overstated.
//
// That is not cosmetic: GetLowQualityTagQueue selects candidates on
// `usage_count = 0`, so an overstated counter keeps a tag whose last real use was
// deleted out of the moderation queue entirely, and the tag listings order by it.
func (s *ArtistDeleteRefsSuite) TestDeleteArtist_ReleasesTheTagUsageCountItHeld() {
	keptArtist := s.seedArtist("Kept")
	doomed := s.seedArtist("Doomed")
	f := s.seedFixtures()

	s.seedArtistRefRow("entity_tags", keptArtist.ID, f)
	s.seedArtistRefRow("entity_tags", doomed.ID, f)

	var before int
	s.Require().NoError(s.db.Raw(`SELECT usage_count FROM tags WHERE id = ?`, f.tagID).
		Scan(&before).Error)
	s.Require().Equal(2, before)

	s.Require().NoError(s.svc.DeleteArtist(doomed.ID))

	var after int
	s.Require().NoError(s.db.Raw(`SELECT usage_count FROM tags WHERE id = ?`, f.tagID).
		Scan(&after).Error)
	s.Equal(1, after,
		"the deleted artist's tag must be given back to tags.usage_count; the surviving "+
			"artist's must not")
}

// The counter must not be driven negative by an entity whose tags outnumber a
// counter that has already drifted. Nothing recomputes usage_count, so drift is
// the expected state rather than a hypothetical, and a delete is not the place to
// turn it into a negative number that reads as "less than unused".
func (s *ArtistDeleteRefsSuite) TestDeleteArtist_TagUsageCountFloorsAtZero() {
	artist := s.seedArtist("Drifted")
	f := s.seedFixtures()
	s.seedArtistRefRow("entity_tags", artist.ID, f)

	s.Require().NoError(s.db.Exec(`UPDATE tags SET usage_count = 0 WHERE id = ?`, f.tagID).Error)
	s.Require().NoError(s.svc.DeleteArtist(artist.ID))

	var after int
	s.Require().NoError(s.db.Raw(`SELECT usage_count FROM tags WHERE id = ?`, f.tagID).
		Scan(&after).Error)
	s.Equal(0, after, "usage_count must floor at zero rather than going negative")
}

// notification_log carries an artist id the inventory loop CANNOT see: an
// artist show-alert row keys entity_type on its own discriminator
// ('artist_show_alert', with a SHOW in entity_id) and names the artist in
// subject_entity_id, a column the inventory has no concept of.
//
// Left behind, a user's inbox keeps an alert about a band that is no longer in
// the catalog. This is the delete-side twin of repointAlertSubjectEntity, and it
// is the shape a coverage guard over entity_type columns can never catch.
func (s *ArtistDeleteRefsSuite) TestDeleteArtist_SweepsAlertRowsNamingItAsSubject() {
	artist := s.seedArtist("Alerted")
	other := s.seedArtist("Untouched")
	f := s.seedFixtures()

	// entity_id here is a SHOW id, deliberately unrelated to either artist: the
	// point is that the row is found through subject_entity_id or not at all.
	const showID = 4242
	s.Require().NoError(s.db.Exec(`
		INSERT INTO notification_log (user_id, entity_type, entity_id, subject_entity_id, channel)
		VALUES (?, ?, ?, ?, 'email')`,
		f.user.ID, notificationm.NotificationEntityArtistShowAlert, showID, artist.ID).Error)
	s.Require().NoError(s.db.Exec(`
		INSERT INTO notification_log (user_id, entity_type, entity_id, subject_entity_id, channel)
		VALUES (?, ?, ?, ?, 'email')`,
		f.user.ID, notificationm.NotificationEntityArtistShowAlert, showID+1, other.ID).Error)

	s.Require().NoError(s.svc.DeleteArtist(artist.ID))

	var stranded int64
	s.Require().NoError(s.db.Raw(
		`SELECT COUNT(*) FROM notification_log WHERE subject_entity_id = ?`, artist.ID).
		Scan(&stranded).Error)
	s.Zero(stranded, "an alert naming the deleted artist as its subject must go with it")

	var untouched int64
	s.Require().NoError(s.db.Raw(
		`SELECT COUNT(*) FROM notification_log WHERE subject_entity_id = ?`, other.ID).
		Scan(&untouched).Error)
	s.Equal(int64(1), untouched, "another artist's alert must not be swept")
}

// The sweep and the delete are one unit. A refused delete must leave every
// reference exactly where it was — otherwise the endpoint that returns 409
// "artist still has shows" would have already destroyed the caller's bookmarks,
// tags and crate memberships on the way to refusing.
//
// This is the case the pre-PSY-1868 code could not have got wrong, because it
// swept nothing; introducing the sweep is what makes it reachable.
func (s *ArtistDeleteRefsSuite) TestDeleteArtist_RefusedDeleteSweepsNothing() {
	artist := s.seedArtist("Booked")
	f := s.seedFixtures()
	s.seedArtistRefRow("user_bookmarks", artist.ID, f)
	s.seedArtistRefRow("entity_tags", artist.ID, f)

	venue := &catalogm.Venue{Name: "Sweep Hall"}
	s.Require().NoError(s.db.Create(venue).Error)
	show := &catalogm.Show{EventDate: time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC)}
	s.Require().NoError(s.db.Create(show).Error)
	s.Require().NoError(s.db.Exec(
		`INSERT INTO show_artists (show_id, artist_id, position) VALUES (?, ?, 0)`,
		show.ID, artist.ID).Error)

	err := s.svc.DeleteArtist(artist.ID)
	s.Require().Error(err, "an artist with shows must not be deletable")

	s.Equal(int64(1), s.countRefRows("user_bookmarks", "entity_id", artist.ID),
		"a refused delete must not have swept the artist's follows")
	s.Equal(int64(1), s.countRefRows("entity_tags", "entity_id", artist.ID),
		"a refused delete must not have swept the artist's tags")

	var usage int
	s.Require().NoError(s.db.Raw(`SELECT usage_count FROM tags WHERE id = ?`, f.tagID).
		Scan(&usage).Error)
	s.Equal(1, usage, "a refused delete must not have released the tag usage count either")

	var stillThere int64
	s.Require().NoError(s.db.Raw(`SELECT COUNT(*) FROM artists WHERE id = ?`, artist.ID).
		Scan(&stillThere).Error)
	s.Equal(int64(1), stillThere)
}

// The sweep must be undone when the DELETE itself is refused by the database,
// not only when this package refuses it first.
//
// artist_relationships is the one foreign key to artists with NO ON DELETE
// clause (see artistFKColumns), so an artist carrying a relationship row cannot
// be deleted at all — Postgres rejects the DELETE. That is pre-existing
// behaviour and PSY-1868 does not change it, but it becomes newly DANGEROUS with
// a sweep in front of it: without one transaction around both, the sweep would
// have already destroyed the artist's follows, tags and crate memberships before
// the database refused to delete the artist they belonged to. This is the
// partial-delete state the transaction exists to prevent, and it is reachable
// today rather than hypothetically.
func (s *ArtistDeleteRefsSuite) TestDeleteArtist_RollsBackTheSweepWhenTheDeleteIsRefused() {
	artist := s.seedArtist("Related")
	other := s.seedArtist("Partner")
	f := s.seedFixtures()
	s.seedArtistRefRow("user_bookmarks", artist.ID, f)
	s.seedArtistRefRow("collection_items", artist.ID, f)

	// The CHECK requires source < target.
	src, tgt := artist.ID, other.ID
	if tgt < src {
		src, tgt = tgt, src
	}
	s.Require().NoError(s.db.Exec(`
		INSERT INTO artist_relationships (source_artist_id, target_artist_id, relationship_type, auto_derived)
		VALUES (?, ?, 'related', false)`, src, tgt).Error)

	err := s.svc.DeleteArtist(artist.ID)
	s.Require().Error(err,
		"artist_relationships has no ON DELETE clause, so the database refuses this delete")

	s.Equal(int64(1), s.countRefRows("user_bookmarks", "entity_id", artist.ID),
		"the sweep must roll back with the refused delete, not leave the artist stripped")
	s.Equal(int64(1), s.countRefRows("collection_items", "entity_id", artist.ID),
		"the sweep must roll back with the refused delete, not leave the artist stripped")

	var stillThere int64
	s.Require().NoError(s.db.Raw(`SELECT COUNT(*) FROM artists WHERE id = ?`, artist.ID).
		Scan(&stillThere).Error)
	s.Equal(int64(1), stillThere, "the artist must survive its own failed delete")
}

// ──────────────────────────────────────────────
// The other five delete paths
// ──────────────────────────────────────────────

// Every Delete* method must sweep, and must sweep for ITS OWN entity type.
//
// Artist has the exhaustive seeding sweep above; the other five had the sweep
// wired and nothing exercising it, which is the shape where a copy-pasted
// `entityTypeArtist` in DeleteLabel ships silently: the sweep runs, deletes
// nothing, every test passes, and every label delete strands its references.
//
// So each path gets one row of a dropped table and one of a kept table, stamped
// with that entity's OWN type, plus a decoy row of the same table stamped with a
// DIFFERENT entity type and the same id. The decoy is what makes the wrong
// constant fail: without it, sweeping the wrong entity_type is indistinguishable
// from sweeping the right one when only one entity exists.
func (s *ArtistDeleteRefsSuite) TestEveryDeletePathSweepsItsOwnEntityType() {
	for _, tc := range []struct {
		entity polymorphicEntityType
		// seedAndDelete creates the entity, returns its id and a func that deletes
		// it through the real service method.
		seedAndDelete func(f artistRefFixtures) (uint, func() error)
	}{
		{
			entity: entityTypeVenue,
			seedAndDelete: func(artistRefFixtures) (uint, func() error) {
				v := &catalogm.Venue{Name: fmt.Sprintf("Sweep Venue %d", time.Now().UnixNano())}
				s.Require().NoError(s.db.Create(v).Error)
				svc := &VenueService{db: s.db}
				return v.ID, func() error { return svc.DeleteVenue(v.ID) }
			},
		},
		{
			entity: entityTypeLabel,
			seedAndDelete: func(artistRefFixtures) (uint, func() error) {
				slug := fmt.Sprintf("sweep-label-%d", time.Now().UnixNano())
				l := &catalogm.Label{Name: "Sweep Label", Slug: &slug}
				s.Require().NoError(s.db.Create(l).Error)
				svc := &LabelService{db: s.db}
				return l.ID, func() error { return svc.DeleteLabel(l.ID) }
			},
		},
		{
			entity: entityTypeFestival,
			seedAndDelete: func(artistRefFixtures) (uint, func() error) {
				unique := time.Now().UnixNano()
				fest := &catalogm.Festival{
					Name:        "Sweep Fest",
					Slug:        fmt.Sprintf("sweep-fest-%d", unique),
					SeriesSlug:  fmt.Sprintf("sweep-fest-series-%d", unique),
					EditionYear: 2026,
					// DATE columns: the zero value is "" and Postgres rejects it.
					StartDate: "2026-09-01",
					EndDate:   "2026-09-03",
				}
				s.Require().NoError(s.db.Create(fest).Error)
				svc := &FestivalService{db: s.db}
				return fest.ID, func() error { return svc.DeleteFestival(fest.ID) }
			},
		},
		{
			entity: entityTypeRelease,
			seedAndDelete: func(artistRefFixtures) (uint, func() error) {
				slug := fmt.Sprintf("sweep-release-%d", time.Now().UnixNano())
				r := &catalogm.Release{Title: "Sweep Release", Slug: &slug}
				s.Require().NoError(s.db.Create(r).Error)
				svc := &ReleaseService{db: s.db}
				return r.ID, func() error { return svc.DeleteRelease(r.ID) }
			},
		},
		{
			entity: entityTypeShow,
			seedAndDelete: func(artistRefFixtures) (uint, func() error) {
				sh := &catalogm.Show{EventDate: time.Date(2026, 9, 2, 3, 0, 0, 0, time.UTC)}
				s.Require().NoError(s.db.Create(sh).Error)
				svc := &ShowService{db: s.db}
				return sh.ID, func() error { return svc.DeleteShow(sh.ID) }
			},
		},
	} {
		s.Run(string(tc.entity), func() {
			f := s.seedFixtures()
			id, deleteIt := tc.seedAndDelete(f)

			// A dropped table and a kept table for this entity type.
			s.Require().NoError(s.db.Exec(
				`INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action)
				 VALUES (?, ?, ?, 'follow')`, f.user.ID, string(tc.entity), id).Error)
			s.Require().NoError(s.db.Exec(
				`INSERT INTO audit_logs (action, entity_type, entity_id) VALUES ('update', ?, ?)`,
				string(tc.entity), id).Error)

			// The decoy: same table, same id, a DIFFERENT entity type. A sweep run
			// with the wrong constant would take this and leave the real one.
			decoyType := "scene"
			s.Require().NoError(s.db.Exec(
				`INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action)
				 VALUES (?, ?, ?, 'follow')`, f.user.ID, decoyType, id).Error)

			s.Require().NoError(deleteIt())

			var swept int64
			s.Require().NoError(s.db.Raw(
				`SELECT COUNT(*) FROM user_bookmarks WHERE entity_type = ? AND entity_id = ?`,
				string(tc.entity), id).Scan(&swept).Error)
			s.Zerof(swept, "the %s delete path did not sweep its own user_bookmarks rows", tc.entity)

			// Scoped to THIS subtest's user: the subtests share one SetupTest, and
			// ids restart per table, so a bare (entity_type, entity_id) count would
			// also match the decoys left by earlier subtests with the same id.
			var decoy int64
			s.Require().NoError(s.db.Raw(
				`SELECT COUNT(*) FROM user_bookmarks
				  WHERE user_id = ? AND entity_type = ? AND entity_id = ?`,
				f.user.ID, decoyType, id).Scan(&decoy).Error)
			s.Equalf(int64(1), decoy,
				"the %s delete path swept rows belonging to another entity type", tc.entity)

			var kept int64
			s.Require().NoError(s.db.Raw(
				`SELECT COUNT(*) FROM audit_logs WHERE entity_type = ? AND entity_id = ?`,
				string(tc.entity), id).Scan(&kept).Error)
			s.Equalf(int64(1), kept,
				"the %s delete path destroyed an audit_logs row that is dispositioned to stay",
				tc.entity)
		})
	}
}

// source_configs is the table the venue and label delete comments call the most
// consequential reason for the sweep — a scraper registration whose entity is
// gone — and it is the one table the artist sweep can never exercise, because
// its CHECK admits only venue and label.
func (s *ArtistDeleteRefsSuite) TestVenueAndLabelDeletesSweepSourceConfigs() {
	venue := &catalogm.Venue{Name: fmt.Sprintf("Scraped Venue %d", time.Now().UnixNano())}
	s.Require().NoError(s.db.Create(venue).Error)
	slug := fmt.Sprintf("scraped-label-%d", time.Now().UnixNano())
	label := &catalogm.Label{Name: "Scraped Label", Slug: &slug}
	s.Require().NoError(s.db.Create(label).Error)

	insert := `INSERT INTO source_configs (entity_type, entity_id, source_url) VALUES (?, ?, ?)`
	s.Require().NoError(s.db.Exec(insert, "venue", venue.ID, "https://example.test/venue").Error)
	s.Require().NoError(s.db.Exec(insert, "label", label.ID, "https://example.test/label").Error)

	s.Require().NoError((&VenueService{db: s.db}).DeleteVenue(venue.ID))
	s.Require().NoError((&LabelService{db: s.db}).DeleteLabel(label.ID))

	var left int64
	s.Require().NoError(s.db.Raw(`SELECT COUNT(*) FROM source_configs`).Scan(&left).Error)
	s.Zero(left, "a scraper registration must not outlive the venue or label it points at")
}

// The alert rows whose entity_id is NOT the entity their name suggests: an
// artist_show_alert holds a SHOW id and a venue_show_alert holds a VENUE id, so
// these are swept by the show and venue delete paths respectively. The
// inventory's own entity_type predicate cannot see either.
func (s *ArtistDeleteRefsSuite) TestShowAndVenueDeletesSweepAlertRowsKeyedOnTheirOwnType() {
	f := s.seedFixtures()

	show := &catalogm.Show{EventDate: time.Date(2026, 9, 3, 3, 0, 0, 0, time.UTC)}
	s.Require().NoError(s.db.Create(show).Error)
	venue := &catalogm.Venue{Name: fmt.Sprintf("Alerted Venue %d", time.Now().UnixNano())}
	s.Require().NoError(s.db.Create(venue).Error)

	s.Require().NoError(s.db.Exec(`
		INSERT INTO notification_log (user_id, entity_type, entity_id, channel)
		VALUES (?, ?, ?, 'email')`,
		f.user.ID, notificationm.NotificationEntityArtistShowAlert, show.ID).Error)
	s.Require().NoError(s.db.Exec(`
		INSERT INTO notification_log (user_id, entity_type, entity_id, alert_bucket, channel)
		VALUES (?, ?, ?, ?, 'email')`,
		f.user.ID, notificationm.NotificationEntityVenueShowAlert, venue.ID,
		time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)).Error)

	s.Require().NoError((&ShowService{db: s.db}).DeleteShow(show.ID))
	s.Require().NoError((&VenueService{db: s.db}).DeleteVenue(venue.ID))

	var showAlerts, venueAlerts int64
	s.Require().NoError(s.db.Raw(
		`SELECT COUNT(*) FROM notification_log WHERE entity_type = ? AND entity_id = ?`,
		notificationm.NotificationEntityArtistShowAlert, show.ID).Scan(&showAlerts).Error)
	s.Zero(showAlerts, "an artist show alert names a SHOW in entity_id and must go with it")

	s.Require().NoError(s.db.Raw(
		`SELECT COUNT(*) FROM notification_log WHERE entity_type = ? AND entity_id = ?`,
		notificationm.NotificationEntityVenueShowAlert, venue.ID).Scan(&venueAlerts).Error)
	s.Zero(venueAlerts, "a venue show alert names a VENUE in entity_id and must go with it")
}

// Deleting one artist must not touch another's references. The sweep's
// statements are all `entity_type = ? AND entity_id = ?`, so the way this breaks
// is a dropped predicate — which would empty a table rather than a row, and no
// single-entity test would notice.
func (s *ArtistDeleteRefsSuite) TestDeleteArtist_LeavesOtherArtistsReferencesAlone() {
	doomed := s.seedArtist("Doomed")
	survivor := s.seedArtist("Survivor")
	f := s.seedFixtures()

	for table := range entityRefTables() {
		if refsThatCannotHoldAnArtist[table] {
			continue
		}
		s.seedArtistRefRow(table, doomed.ID, f)
		s.seedArtistRefRow(table, survivor.ID, f)
	}

	s.Require().NoError(s.svc.DeleteArtist(doomed.ID))

	for _, ref := range polymorphicEntityRefs {
		if refsThatCannotHoldAnArtist[ref.table] {
			continue
		}
		s.Equalf(int64(1), s.countRefRows(ref.table, ref.idCol, survivor.ID),
			"%s lost the surviving artist's row", ref.table)
	}
	for _, table := range refsRepointedElsewhere {
		s.Equalf(int64(1), s.countRefRows(table, refsRepointedElsewhereIDCol, survivor.ID),
			"%s lost the surviving artist's row", table)
	}
}
