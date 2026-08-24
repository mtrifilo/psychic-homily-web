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
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// PSY-559: Show dedup integration tests
// =============================================================================

type ShowDedupTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *ShowDedupTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *ShowDedupTestSuite) TearDownSuite() {
	if s.testDB != nil {
		s.testDB.Cleanup()
	}
}

func (s *ShowDedupTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	for _, t := range []string{
		"comment_subscriptions", "comment_votes", "comment_edits", "comment_last_read", "comments",
		"tag_votes", "entity_tags", "tags", "entity_reports", "pending_entity_edits",
		"entity_edit_audit_logs", "notification_log", "entity_requests",
		"revisions", "requests", "audit_logs", "collection_items", "collections",
		"user_bookmarks", "show_reports", "enrichment_queue",
		"show_artists", "show_venues", "shows", "artists", "venues", "users",
	} {
		_, _ = sqlDB.Exec(fmt.Sprintf("DELETE FROM %s", t))
	}
}

func TestShowDedupTestSuite(t *testing.T) {
	suite.Run(t, new(ShowDedupTestSuite))
}

// --- helpers ---

func (s *ShowDedupTestSuite) seedUser(email string) *authm.User {
	u := &authm.User{
		Email:         stringPtr(email),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(u).Error)
	return u
}

func (s *ShowDedupTestSuite) seedArtist(name string) *catalogm.Artist {
	slug := name
	a := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a
}

func (s *ShowDedupTestSuite) seedVenue(name, city, state string) *catalogm.Venue {
	slug := name
	v := &catalogm.Venue{Name: name, Slug: &slug, City: city, State: state, Verified: true}
	s.Require().NoError(s.db.Create(v).Error)
	return v
}

// seedShow inserts a show with the given event_date, links artist as
// headliner and venue. Uses raw SQL so we control created_at exactly.
func (s *ShowDedupTestSuite) seedShow(title string, eventDate, createdAt time.Time, artistID, venueID uint, state string) uint {
	var id uint
	row := s.db.Raw(`
		INSERT INTO shows (title, event_date, state, status, source, created_at, updated_at, slug)
		VALUES (?, ?, ?, 'approved', 'user', ?, ?, ?)
		RETURNING id
	`, title, eventDate, state, createdAt, createdAt, fmt.Sprintf("%s-%d", title, eventDate.Unix())).Row()
	s.Require().NoError(row.Scan(&id))

	s.Require().NoError(s.db.Exec(
		`INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')`,
		id, artistID).Error)
	s.Require().NoError(s.db.Exec(
		`INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)`,
		id, venueID).Error)
	return id
}

// --- tests ---

// TestFindClusters_BasicPair confirms two shows with the same
// (artist, venue, event_date) are detected as a duplicate cluster.
func (s *ShowDedupTestSuite) TestFindClusters_BasicPair() {
	a := s.seedArtist("Peter Hook")
	v := s.seedVenue("The Van Buren", "Phoenix", "AZ")
	eventDate := time.Date(2026, 9, 16, 2, 30, 0, 0, time.UTC) // 7:30pm Phoenix on Sept 15
	earlier := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	later := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

	id1 := s.seedShow("Peter Hook 1", eventDate, earlier, a.ID, v.ID, "AZ")
	id2 := s.seedShow("Peter Hook 2", eventDate, later, a.ID, v.ID, "AZ")

	clusters, err := FindShowDedupClusters(s.db)
	s.Require().NoError(err)
	s.Require().Len(clusters, 1)

	c := clusters[0]
	s.Equal(a.ID, c.Key.ArtistID)
	s.Equal(v.ID, c.Key.VenueID)
	s.True(eventDate.Equal(c.Key.EventDate))
	s.Equal(id1, c.WinnerID)
	s.Equal([]uint{id2}, c.LoserIDs)
}

// TestFindClusters_MatineeAndEvening — the matinee+evening exception
// case from the ticket. Same artist + same venue on the same DATE
// but DIFFERENT event_date timestamps must NOT be collapsed.
func (s *ShowDedupTestSuite) TestFindClusters_MatineeAndEvening() {
	a := s.seedArtist("Just Mustard")
	v := s.seedVenue("Valley Bar", "Phoenix", "AZ")
	matinee := time.Date(2026, 5, 17, 20, 0, 0, 0, time.UTC) // 1pm AZ
	evening := time.Date(2026, 5, 18, 3, 0, 0, 0, time.UTC)  // 8pm AZ
	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	_ = s.seedShow("Matinee", matinee, created, a.ID, v.ID, "AZ")
	_ = s.seedShow("Evening", evening, created, a.ID, v.ID, "AZ")

	clusters, err := FindShowDedupClusters(s.db)
	s.Require().NoError(err)
	s.Empty(clusters, "matinee+evening at same venue must not be collapsed")
}

// TestFindClusters_DifferentVenues confirms shows with the same artist
// + event_date but different venues are NOT clustered.
func (s *ShowDedupTestSuite) TestFindClusters_DifferentVenues() {
	a := s.seedArtist("Amyl And The Sniffers")
	v1 := s.seedVenue("Van Buren", "Phoenix", "AZ")
	v2 := s.seedVenue("Crescent Ballroom", "Phoenix", "AZ")
	eventDate := time.Date(2026, 4, 11, 2, 30, 0, 0, time.UTC)
	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	_ = s.seedShow("Show A", eventDate, created, a.ID, v1.ID, "AZ")
	_ = s.seedShow("Show B", eventDate, created, a.ID, v2.ID, "AZ")

	clusters, err := FindShowDedupClusters(s.db)
	s.Require().NoError(err)
	s.Empty(clusters)
}

// TestMergeDuplicateShow_BasicMerge runs the full merge and confirms
// the loser is gone, the winner is preserved.
func (s *ShowDedupTestSuite) TestMergeDuplicateShow_BasicMerge() {
	a := s.seedArtist("Headliner")
	v := s.seedVenue("Hall", "Phoenix", "AZ")
	eventDate := time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	winner := s.seedShow("First", eventDate, t1, a.ID, v.ID, "AZ")
	loser := s.seedShow("Second", eventDate, t2, a.ID, v.ID, "AZ")

	summary := &ShowDedupSummary{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		return MergeDuplicateShow(tx, winner, loser, summary)
	})
	s.Require().NoError(err)

	// Winner survives.
	var winnerCount int64
	s.db.Model(&catalogm.Show{}).Where("id = ?", winner).Count(&winnerCount)
	s.Equal(int64(1), winnerCount)

	// Loser deleted.
	var loserCount int64
	s.db.Model(&catalogm.Show{}).Where("id = ?", loser).Count(&loserCount)
	s.Equal(int64(0), loserCount)

	// show_artists / show_venues junctions still cover the winner.
	var saCount, svCount int64
	s.db.Table("show_artists").Where("show_id = ?", winner).Count(&saCount)
	s.db.Table("show_venues").Where("show_id = ?", winner).Count(&svCount)
	s.Equal(int64(1), saCount)
	s.Equal(int64(1), svCount)

	s.Equal(1, summary.LosersMerged)
}

// TestMergeDuplicateShow_RepointsBookmarks confirms a bookmark on the
// loser is repointed to the winner, with conflicts dropped.
func (s *ShowDedupTestSuite) TestMergeDuplicateShow_RepointsBookmarks() {
	a := s.seedArtist("X")
	v := s.seedVenue("Y", "Phoenix", "AZ")
	eventDate := time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC)
	winner := s.seedShow("W", eventDate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	loser := s.seedShow("L", eventDate, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")

	u1 := s.seedUser("a@test.com")
	u2 := s.seedUser("b@test.com")

	// u1 has a 'save' on the winner already — loser's save by u1
	// must be dropped on conflict.
	insertBookmark := `INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action) VALUES (?, 'show', ?, 'save')`
	s.Require().NoError(s.db.Exec(insertBookmark, u1.ID, winner).Error)
	s.Require().NoError(s.db.Exec(insertBookmark, u1.ID, loser).Error)
	s.Require().NoError(s.db.Exec(insertBookmark, u2.ID, loser).Error) // no conflict

	summary := &ShowDedupSummary{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		return MergeDuplicateShow(tx, winner, loser, summary)
	})
	s.Require().NoError(err)

	// u1 still has exactly one save on winner; u2 also has one.
	var u1Count, u2Count int64
	s.db.Table("user_bookmarks").
		Where("user_id = ? AND entity_type = 'show' AND entity_id = ? AND action = 'save'", u1.ID, winner).
		Count(&u1Count)
	s.db.Table("user_bookmarks").
		Where("user_id = ? AND entity_type = 'show' AND entity_id = ? AND action = 'save'", u2.ID, winner).
		Count(&u2Count)
	s.Equal(int64(1), u1Count)
	s.Equal(int64(1), u2Count)

	// Nothing left pointing at the loser.
	var loserCount int64
	s.db.Table("user_bookmarks").
		Where("entity_type = 'show' AND entity_id = ?", loser).
		Count(&loserCount)
	s.Equal(int64(0), loserCount)

	s.Equal(int64(1), summary.EntityRefsMoved["user_bookmarks"])
	s.Equal(int64(1), summary.EntityRefsDropped["user_bookmarks"])
}

// TestMergeDuplicateShow_RepointsCollectionItems confirms collection
// items are repointed and the unique-per-collection constraint is
// honoured.
func (s *ShowDedupTestSuite) TestMergeDuplicateShow_RepointsCollectionItems() {
	a := s.seedArtist("X")
	v := s.seedVenue("Y", "Phoenix", "AZ")
	eventDate := time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC)
	winner := s.seedShow("W", eventDate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	loser := s.seedShow("L", eventDate, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")

	u := s.seedUser("c@test.com")

	// Create a collection. Use only the columns required by NOT NULL.
	var collectionID uint
	row := s.db.Raw(`
		INSERT INTO collections (title, slug, creator_id)
		VALUES ('Test', ?, ?)
		RETURNING id
	`, fmt.Sprintf("test-%d", time.Now().UnixNano()), u.ID).Row()
	s.Require().NoError(row.Scan(&collectionID))

	// One item on winner already, one item on loser → conflict drop.
	insertItem := `INSERT INTO collection_items (collection_id, entity_type, entity_id, position, added_by_user_id) VALUES (?, 'show', ?, 0, ?)`
	s.Require().NoError(s.db.Exec(insertItem, collectionID, winner, u.ID).Error)
	s.Require().NoError(s.db.Exec(insertItem, collectionID, loser, u.ID).Error)

	summary := &ShowDedupSummary{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		return MergeDuplicateShow(tx, winner, loser, summary)
	})
	s.Require().NoError(err)

	var n int64
	s.db.Table("collection_items").
		Where("collection_id = ? AND entity_type = 'show' AND entity_id = ?", collectionID, winner).
		Count(&n)
	s.Equal(int64(1), n, "exactly one item should remain on winner per collection")
	s.Equal(int64(1), summary.EntityRefsDropped["collection_items"])
}

// The show dedup CLI runs with no admin in the loop, so its revision re-point
// goes through the shared helper and has to state a provenance decision. The
// decision is per merge and turns on the LOSER's status, because the merge
// deletes the row the read-time visibility gate would have consulted
// (PSY-1715).
//
// An approved loser was hiding nothing, so its rows arrive re-pointed, counted
// and UNSTAMPED. The gated case is its twin below, and the pair is what keeps
// the stamp from degenerating into "always" or "never".
func (s *ShowDedupTestSuite) TestMergeDuplicateShow_ApprovedLoserRevisionsAreUnstamped() {
	a := s.seedArtist("Repoint")
	v := s.seedVenue("Repoint Hall", "Phoenix", "AZ")
	eventDate := time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC)
	winner := s.seedShow("W", eventDate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	loser := s.seedShow("L", eventDate, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	u := s.seedUser("revisions@test.com")

	revisionID := seedRevision(s.T(), s.db, "show", loser, u.ID, "fixed the door time")

	summary := &ShowDedupSummary{}
	s.Require().NoError(s.db.Transaction(func(tx *gorm.DB) error {
		return MergeDuplicateShow(tx, winner, loser, summary)
	}))

	var moved adminm.Revision
	s.Require().NoError(s.db.First(&moved, revisionID).Error)
	s.Equal(winner, moved.EntityID, "the revision must be re-pointed at the surviving show")
	s.False(moved.FromGatedShow,
		"an approved loser was suppressing nothing, so its history must not be stamped")
	s.False(moved.FromUnverifiedVenue,
		"a show merge must not stamp the venue redaction marker on show history")
	s.Equal(int64(1), summary.RevisionsMoved,
		"the helper's row count must still reach the dedup summary the CLI reports")
}

// The leak this stamp closes, end to end at the merge. FindShowDedupClusters
// selects losers from status IN ('approved','private'), so merging an
// unpublished show into an approved one is a path the CLI takes. Without the
// stamp, the loser's history lands on a show whose status says "publish this"
// and every field it recorded becomes world-readable.
func (s *ShowDedupTestSuite) TestMergeDuplicateShow_GatedLoserRevisionsAreStamped() {
	a := s.seedArtist("Gated Repoint")
	v := s.seedVenue("Gated Hall", "Phoenix", "AZ")
	eventDate := time.Date(2026, 7, 1, 3, 0, 0, 0, time.UTC)
	winner := s.seedShow("GW", eventDate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	loser := s.seedShow("GL", eventDate, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	u := s.seedUser("gated-revisions@test.com")

	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", loser).
		Update("status", catalogm.ShowStatusPrivate).Error)

	revisionID := seedRevision(s.T(), s.db, "show", loser, u.ID, "moved it to the house")

	summary := &ShowDedupSummary{}
	s.Require().NoError(s.db.Transaction(func(tx *gorm.DB) error {
		return MergeDuplicateShow(tx, winner, loser, summary)
	}))

	var moved adminm.Revision
	s.Require().NoError(s.db.First(&moved, revisionID).Error)
	s.Equal(winner, moved.EntityID, "the revision must still be re-pointed at the surviving show")
	s.True(moved.FromGatedShow,
		"a private loser's history must stay suppressed after landing on an approved winner")
	s.Equal(int64(1), summary.RevisionsMoved)

	// The stamp is a marker, not a scrub. Rollback restores stored values, so
	// the diff has to survive the merge intact.
	s.Require().NotNil(moved.FieldChanges)
	s.Contains(string(*moved.FieldChanges), "old_value",
		"the stored diff must survive the stamp — rollback reads it")
}

// The case that makes the stamp depend on the WINNER too. FindShowDedupClusters
// selects candidates from status IN ('approved','private'), so both members of a
// cluster can be private, and POST /shows/{id}/publish is a first-class user
// action on the survivor.
//
// Stamping here would be permanent: the mark only ever goes TRUE and nothing
// clears it, so publishing the winner would leave a fully public show whose
// carried edit history stayed invisible to everyone but admins, forever. The
// rows must move UNSTAMPED instead, so they keep tracking the winner's status
// the way they would have if the two shows had always been one row.
func (s *ShowDedupTestSuite) TestMergeDuplicateShow_GatedWinnerDoesNotStampPermanently() {
	a := s.seedArtist("Both Private")
	v := s.seedVenue("Private Hall", "Phoenix", "AZ")
	eventDate := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	winner := s.seedShow("PW", eventDate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	loser := s.seedShow("PL", eventDate, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	u := s.seedUser("both-private@test.com")

	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id IN ?", []uint{winner, loser}).
		Update("status", catalogm.ShowStatusPrivate).Error)

	revisionID := seedRevision(s.T(), s.db, "show", loser, u.ID, "fixed the set times")

	summary := &ShowDedupSummary{}
	s.Require().NoError(s.db.Transaction(func(tx *gorm.DB) error {
		return MergeDuplicateShow(tx, winner, loser, summary)
	}))

	var moved adminm.Revision
	s.Require().NoError(s.db.First(&moved, revisionID).Error)
	s.Equal(winner, moved.EntityID)
	s.False(moved.FromGatedShow,
		"a gated winner already suppresses these rows by status; stamping them would "+
			"outlive the reason for it and survive the show being published")

	// The property that makes it matter: publish the winner, and the carried
	// history comes back. With a stamp it never would.
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", winner).
		Update("status", catalogm.ShowStatusApproved).Error)

	var afterPublish adminm.Revision
	s.Require().NoError(s.db.First(&afterPublish, revisionID).Error)
	s.False(afterPublish.FromGatedShow,
		"publishing the surviving show must restore the merged history, not leave it suppressed")
}

// seedShowPendingEdit records one proposed edit against a show. Written through
// the model so a column rename breaks the build rather than the test run.
func (s *ShowDedupTestSuite) seedShowPendingEdit(showID, userID uint) uint {
	changes := json.RawMessage(`[{"field":"title","old_value":"before","new_value":"after"}]`)
	edit := &adminm.PendingEntityEdit{
		EntityType:   "show",
		EntityID:     showID,
		SubmittedBy:  userID,
		FieldChanges: &changes,
		Summary:      "title fix",
		Status:       adminm.PendingEditStatusPending,
	}
	s.Require().NoError(s.db.Create(edit).Error)
	return edit.ID
}

// Same forcing function as revisions, one table over: this CLI deletes the show
// a read-time gate would consult, so its pending-edit re-point has to state a
// provenance decision rather than being a raw UPDATE in a list.
//
// It also fixes what the raw UPDATE got wrong. idx_pending_entity_edits_unique
// is UNIQUE on (entity_type, entity_id, submitted_by) WHERE status = 'pending',
// which the old "no uniqueness constraint" grouping did not account for — one
// contributor with a pending edit on both shows aborted the entire dedup
// transaction. The helper drops the loser's row first, which is this merge's
// documented conflict policy.
func (s *ShowDedupTestSuite) TestMergeDuplicateShow_PendingEditsRepointAndDedupe() {
	a := s.seedArtist("Pending")
	v := s.seedVenue("Pending Hall", "Phoenix", "AZ")
	eventDate := time.Date(2026, 7, 1, 3, 0, 0, 0, time.UTC)
	winner := s.seedShow("W", eventDate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	loser := s.seedShow("L", eventDate, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")

	bothShows := s.seedUser("prolific@test.com")
	loserOnly := s.seedUser("newcomer@test.com")

	kept := s.seedShowPendingEdit(winner, bothShows.ID)
	// Before the helper, re-pointing this row alongside `kept` violated the
	// partial unique index and rolled the whole merge back.
	dropped := s.seedShowPendingEdit(loser, bothShows.ID)
	survives := s.seedShowPendingEdit(loser, loserOnly.ID)

	summary := &ShowDedupSummary{}
	s.Require().NoError(s.db.Transaction(func(tx *gorm.DB) error {
		return MergeDuplicateShow(tx, winner, loser, summary)
	}))

	var droppedCount int64
	s.Require().NoError(s.db.Model(&adminm.PendingEntityEdit{}).
		Where("id = ?", dropped).Count(&droppedCount).Error)
	s.Zero(droppedCount, "the colliding loser edit must be dropped, not re-pointed into a "+
		"unique-index violation that aborts the dedup run")

	var keptRow, survivingRow adminm.PendingEntityEdit
	s.Require().NoError(s.db.First(&keptRow, kept).Error)
	s.Equal(winner, keptRow.EntityID, "the winner's own pending edit must be untouched")
	s.Require().NoError(s.db.First(&survivingRow, survives).Error)
	s.Equal(winner, survivingRow.EntityID,
		"a contributor with no competing edit on the winner keeps theirs")

	var stale int64
	s.Require().NoError(s.db.Model(&adminm.PendingEntityEdit{}).
		Where("entity_type = 'show' AND entity_id = ?", loser).Count(&stale).Error)
	s.Zero(stale, "pending_entity_edits left a dangling reference to the deleted show")

	s.Equal(int64(1), summary.PendingEditsMoved,
		"the helper's row count must still reach the dedup summary the CLI reports")
	s.Equal(int64(1), summary.PendingEditsSkipped,
		"a dropped contributor edit must be visible in the CLI summary, not silent")
}

// TestRecanonicaliseShowSlug rewrites a legacy (UTC-derived) slug to
// the venue-timezone-aware canonical form.
func (s *ShowDedupTestSuite) TestRecanonicaliseShowSlug() {
	a := s.seedArtist("Peter Hook")
	v := s.seedVenue("The Van Buren", "Phoenix", "AZ")

	// 7:30pm Phoenix on Sept 15 = 02:30 UTC on Sept 16. Legacy
	// migration-000019 slug used UTC date → "…2026-09-16".
	eventDate := time.Date(2026, 9, 16, 2, 30, 0, 0, time.UTC)
	id := s.seedShow("Peter Hook", eventDate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")

	// Force a legacy-style slug.
	legacy := "peter-hook-and-the-light-at-the-van-buren-2026-09-16"
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", id).Update("slug", legacy).Error)

	rewritten, err := RecanonicaliseShowSlug(s.db, id)
	s.Require().NoError(err)
	s.True(rewritten)

	var got catalogm.Show
	s.Require().NoError(s.db.First(&got, id).Error)
	s.Require().NotNil(got.Slug)
	// Canonical form puts the venue-local date FIRST.
	s.Contains(*got.Slug, "2026-09-15")
	s.Contains(*got.Slug, "at-the-van-buren")
}

// TestDedupChetFakerPair_LegacyAndCanonicalSlugs_PSY571 locks in the
// end-to-end behaviour for the Chet Faker shape from PSY-571: two
// shows share the same (artist, venue, event_date) but carry
// different slug forms — the older record has the legacy migration-
// 000019 UTC-derived slug ("…YYYY-MM-DD"), the newer record has the
// canonical venue-local-date-first slug ("YYYY-MM-DD-…").
//
// The full dedup pass must:
//  1. detect the pair as a single cluster (existing key catches it);
//  2. merge the loser into the winner (older record wins by created_at);
//  3. recanonicalise the surviving record's slug to the canonical form.
//
// After the pass exactly one show remains, with the canonical slug.
func (s *ShowDedupTestSuite) TestDedupChetFakerPair_LegacyAndCanonicalSlugs_PSY571() {
	a := s.seedArtist("Chet Faker")
	v := s.seedVenue("The Van Buren", "Phoenix", "AZ")

	// 8pm Phoenix on May 3 = 03:00 UTC on May 4. Same event_date for
	// both records — only the slugs differ.
	eventDate := time.Date(2026, 5, 4, 3, 0, 0, 0, time.UTC)
	earlier := time.Date(2025, 11, 29, 0, 0, 0, 0, time.UTC)
	later := time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC)

	// seedShow generates a slug from title — give the two records
	// distinct titles so initial inserts don't collide on the unique
	// index, then overwrite each slug below.
	winnerID := s.seedShow("Chet Faker (legacy)", eventDate, earlier, a.ID, v.ID, "AZ")
	loserID := s.seedShow("Chet Faker (canonical)", eventDate, later, a.ID, v.ID, "AZ")

	// Force the legacy + canonical slug pairing seen in production.
	legacySlug := "chet-faker-at-the-van-buren-2026-05-04"
	canonicalSlug := "2026-05-03-chet-faker-at-the-van-buren"
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", winnerID).Update("slug", legacySlug).Error)
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", loserID).Update("slug", canonicalSlug).Error)

	clusters, err := FindShowDedupClusters(s.db)
	s.Require().NoError(err)
	s.Require().Len(clusters, 1)
	s.Equal(winnerID, clusters[0].WinnerID, "older record should win")
	s.Equal([]uint{loserID}, clusters[0].LoserIDs)

	// Mirror the dedup-shows cmd's per-cluster transaction: merge
	// losers, then recanonicalise the winner's slug.
	summary := &ShowDedupSummary{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := MergeDuplicateShow(tx, winnerID, loserID, summary); err != nil {
			return err
		}
		_, err := RecanonicaliseShowSlug(tx, winnerID)
		return err
	})
	s.Require().NoError(err)

	// Loser is gone, exactly one Chet Faker show remains.
	var remaining int64
	s.db.Model(&catalogm.Show{}).
		Joins("JOIN show_artists sa ON sa.show_id = shows.id").
		Joins("JOIN show_venues  sv ON sv.show_id = shows.id").
		Where("sa.artist_id = ? AND sv.venue_id = ? AND shows.event_date = ?", a.ID, v.ID, eventDate).
		Count(&remaining)
	s.Equal(int64(1), remaining, "exactly one Chet Faker show should remain post-merge")

	// Surviving slug is the canonical venue-local form.
	var got catalogm.Show
	s.Require().NoError(s.db.First(&got, winnerID).Error)
	s.Require().NotNil(got.Slug)
	s.Equal(canonicalSlug, *got.Slug,
		"winner's slug should be recanonicalised to the venue-local-date-first form")
}

// PSY-1896: follow-driven alert rows carry a show id in notification_log.
// entity_id under their OWN entity_type, which polymorphicEntityRefs cannot see,
// so a merge that ignores them strands a user's inbox row against a deleted
// show and it renders with no title and no link.
func (s *ShowDedupTestSuite) TestMergeShow_MovesArtistShowAlertRows() {
	u := s.seedUser("alerts-move@test.local")
	a := s.seedArtist("Alert Band")
	v := s.seedVenue("Alert Hall", "Phoenix", "AZ")
	eventDate := time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC)

	winner := s.seedShow("Winner", eventDate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	loser := s.seedShow("Loser", eventDate, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")

	// The user was alerted about the LOSER only.
	s.Require().NoError(s.db.Exec(`
		INSERT INTO notification_log (user_id, entity_type, entity_id, subject_entity_id, channel, sent_at)
		VALUES (?, 'artist_show_alert', ?, ?, 'in_app', now())`, u.ID, loser, a.ID).Error)

	summary := &ShowDedupSummary{}
	s.Require().NoError(s.db.Transaction(func(tx *gorm.DB) error {
		return MergeDuplicateShow(tx, winner, loser, summary)
	}))

	var onWinner int64
	s.Require().NoError(s.db.Table("notification_log").
		Where("entity_type = 'artist_show_alert' AND entity_id = ?", winner).
		Count(&onWinner).Error)
	s.Equal(int64(1), onWinner, "the alert must follow the show it is about")
	s.Equal(int64(1), summary.AlertRowsMoved)
}

// The drop half: uq_notification_log_artist_show_alert is UNIQUE on
// (user_id, entity_id, channel) for this discriminator, so a user alerted about
// BOTH shows before they were found to be duplicates cannot have the loser's row
// moved onto the winner. Without the dedupe the whole merge aborts on the
// constraint.
func (s *ShowDedupTestSuite) TestMergeShow_DropsConflictingArtistShowAlertRows() {
	u := s.seedUser("alerts-conflict@test.local")
	a := s.seedArtist("Conflict Band")
	v := s.seedVenue("Conflict Hall", "Phoenix", "AZ")
	eventDate := time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC)

	winner := s.seedShow("Winner", eventDate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")
	loser := s.seedShow("Loser", eventDate, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), a.ID, v.ID, "AZ")

	for _, showID := range []uint{winner, loser} {
		s.Require().NoError(s.db.Exec(`
			INSERT INTO notification_log (user_id, entity_type, entity_id, subject_entity_id, channel, sent_at)
			VALUES (?, 'artist_show_alert', ?, ?, 'in_app', now())`, u.ID, showID, a.ID).Error)
	}

	summary := &ShowDedupSummary{}
	s.Require().NoError(s.db.Transaction(func(tx *gorm.DB) error {
		return MergeDuplicateShow(tx, winner, loser, summary)
	}))

	var onWinner int64
	s.Require().NoError(s.db.Table("notification_log").
		Where("entity_type = 'artist_show_alert' AND entity_id = ? AND user_id = ?", winner, u.ID).
		Count(&onWinner).Error)
	s.Equal(int64(1), onWinner,
		"the redundant half of a duplicate notification the user already saw is dropped, not stacked")
}
