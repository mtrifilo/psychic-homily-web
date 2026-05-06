package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

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
		"comment_subscriptions", "comment_votes", "comment_edits", "comments",
		"entity_tags", "entity_reports", "pending_entity_edits",
		"revisions", "requests", "audit_logs", "collection_items",
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

	s.Equal(int64(1), summary.BookmarksMoved)
	s.Equal(int64(1), summary.BookmarksSkipped)
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
	s.Equal(int64(1), summary.CollectionsSkipped)
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
