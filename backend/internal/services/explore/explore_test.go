package explore

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	adminsvc "psychic-homily-backend/internal/services/admin"
	"psychic-homily-backend/internal/testutil"
)

// ExploreServiceIntegrationSuite drives the /explore reads against a
// real Postgres testcontainer. The shape under test — chronological
// ordering, polymorphic-referent defensive reads, ±90d shuffle pool
// membership — only exercises end-to-end against actual rows + joins.
// Pure-unit mocking the GORM layer would assert syntax, not behaviour.
type ExploreServiceIntegrationSuite struct {
	suite.Suite
	testDB              *testutil.TestDatabase
	db                  *gorm.DB
	featuredSlotService *adminsvc.FeaturedSlotService
	exploreService      *ExploreService
}

func (s *ExploreServiceIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.featuredSlotService = adminsvc.NewFeaturedSlotService(s.db)
	s.exploreService = NewExploreService(s.db, s.featuredSlotService)
}

func (s *ExploreServiceIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *ExploreServiceIntegrationSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	// Order respects FKs — children before parents. featured_slots
	// references users; collections/shows are independent leaves but
	// they're referenced by show_artists/show_venues children.
	for _, stmt := range []string{
		"DELETE FROM featured_slots",
		// The predicate-parity rejection tests write a rejected-attempt
		// audit row via SetActiveSlot; clean it so any audit-counting test
		// added to this suite later stays order-independent. actor_id is
		// ON DELETE SET NULL so ordering vs. users below doesn't matter.
		"DELETE FROM audit_logs",
		"DELETE FROM show_artists",
		"DELETE FROM show_venues",
		"DELETE FROM shows",
		"DELETE FROM artists",
		"DELETE FROM venues",
		"DELETE FROM collection_items",
		"DELETE FROM collections",
		"DELETE FROM users",
	} {
		_, _ = sqlDB.Exec(stmt)
	}
}

func TestExploreServiceIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ExploreServiceIntegrationSuite))
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

func (s *ExploreServiceIntegrationSuite) createAdmin(label string) *authm.User {
	email := fmt.Sprintf("%s-%d@test.com", label, time.Now().UnixNano())
	user := &authm.User{
		Email:         &email,
		FirstName:     &label,
		IsActive:      true,
		IsAdmin:       true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(user).Error)
	return user
}

// createShow inserts an approved show with one venue + one headliner
// artist, dated relative to NOW(). Returns the show ID + artist ID for
// downstream assertions.
func (s *ExploreServiceIntegrationSuite) createShow(title string, daysFromNow int) (*catalogm.Show, *catalogm.Artist, *catalogm.Venue) {
	city := "Phoenix"
	state := "AZ"
	slug := fmt.Sprintf("show-%s-%d", title, time.Now().UnixNano())
	show := &catalogm.Show{
		Title:     title,
		Slug:      &slug,
		EventDate: time.Now().UTC().AddDate(0, 0, daysFromNow),
		City:      &city,
		State:     &state,
		Status:    catalogm.ShowStatusApproved,
	}
	s.Require().NoError(s.db.Create(show).Error)

	venue := &catalogm.Venue{
		Name:     title + " Venue",
		City:     "Phoenix",
		State:    "AZ",
		Verified: true,
	}
	s.Require().NoError(s.db.Create(venue).Error)

	artistSlug := fmt.Sprintf("artist-%s-%d", title, time.Now().UnixNano())
	artist := &catalogm.Artist{
		Name: title + " Artist",
		Slug: &artistSlug,
	}
	s.Require().NoError(s.db.Create(artist).Error)

	s.db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	s.db.Exec(`INSERT INTO show_artists (show_id, artist_id, position, set_type, event_date, venue_id)
	             VALUES (?, ?, 0, 'headliner', ?, ?)`,
		show.ID, artist.ID, show.EventDate, venue.ID)

	return show, artist, venue
}

// createCollection seeds a collection with the given visibility.
// GORM-bool gotcha (per CLAUDE.md): IsPublic = false is the zero-value
// so Create silently ignores it and the column default (true) wins.
// Insert as public, then Update to flip private when needed.
func (s *ExploreServiceIntegrationSuite) createCollection(creatorID uint, title string, isPublic bool) *communitym.Collection {
	coll := &communitym.Collection{
		Title:     title,
		Slug:      fmt.Sprintf("%s-%d", title, time.Now().UnixNano()),
		CreatorID: creatorID,
		IsPublic:  true,
	}
	s.Require().NoError(s.db.Create(coll).Error)
	if !isPublic {
		s.Require().NoError(s.db.Model(coll).Update("is_public", false).Error)
		coll.IsPublic = false
	}
	return coll
}

// ──────────────────────────────────────────────
// GetUpcomingShows
// ──────────────────────────────────────────────

func (s *ExploreServiceIntegrationSuite) TestGetUpcomingShows_Empty() {
	resp, err := s.exploreService.GetUpcomingShows(20, 0)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Equal(int64(0), resp.Total)
	s.Empty(resp.Shows)
	s.Equal(20, resp.Limit)
}

func (s *ExploreServiceIntegrationSuite) TestGetUpcomingShows_ChronologicalOrder() {
	// Insert in non-chronological order so the test asserts SQL
	// ORDER BY, not insertion order.
	s.createShow("c-far", 30)
	s.createShow("a-soon", 1)
	s.createShow("b-mid", 7)

	resp, err := s.exploreService.GetUpcomingShows(20, 0)
	s.Require().NoError(err)
	s.Require().Len(resp.Shows, 3)
	s.Equal(int64(3), resp.Total)

	// Verify ascending event_date order.
	for i := 1; i < len(resp.Shows); i++ {
		s.True(resp.Shows[i-1].EventDate.Before(resp.Shows[i].EventDate),
			"shows must be in ascending event_date order")
	}
}

func (s *ExploreServiceIntegrationSuite) TestGetUpcomingShows_ExcludesPast() {
	// Past show should not surface even though it's approved.
	s.createShow("past", -3)
	future, _, _ := s.createShow("future", 5)

	resp, err := s.exploreService.GetUpcomingShows(20, 0)
	s.Require().NoError(err)
	s.Require().Len(resp.Shows, 1)
	s.Equal(future.ID, resp.Shows[0].ID)
}

func (s *ExploreServiceIntegrationSuite) TestGetUpcomingShows_ExcludesNonApproved() {
	// Pending + Private + Rejected shows must not leak through.
	city := "Phoenix"
	state := "AZ"
	for _, status := range []catalogm.ShowStatus{
		catalogm.ShowStatusPending, catalogm.ShowStatusPrivate, catalogm.ShowStatusRejected,
	} {
		show := &catalogm.Show{
			Title:     fmt.Sprintf("non-approved %s", status),
			EventDate: time.Now().UTC().AddDate(0, 0, 5),
			City:      &city,
			State:     &state,
			Status:    status,
		}
		s.Require().NoError(s.db.Create(show).Error)
	}
	visible, _, _ := s.createShow("approved", 5)

	resp, err := s.exploreService.GetUpcomingShows(20, 0)
	s.Require().NoError(err)
	s.Require().Len(resp.Shows, 1)
	s.Equal(visible.ID, resp.Shows[0].ID)
}

func (s *ExploreServiceIntegrationSuite) TestGetUpcomingShows_DeterministicPaginationOnTie() {
	// Two shows with identical event_date — pagination must order by
	// id ASC so the page boundary is reproducible across calls.
	eventDate := time.Now().UTC().AddDate(0, 0, 7)
	city := "Phoenix"
	state := "AZ"
	a := &catalogm.Show{Title: "tie-a", EventDate: eventDate, City: &city, State: &state, Status: catalogm.ShowStatusApproved}
	b := &catalogm.Show{Title: "tie-b", EventDate: eventDate, City: &city, State: &state, Status: catalogm.ShowStatusApproved}
	s.Require().NoError(s.db.Create(a).Error)
	s.Require().NoError(s.db.Create(b).Error)

	resp, err := s.exploreService.GetUpcomingShows(10, 0)
	s.Require().NoError(err)
	s.Require().Len(resp.Shows, 2)
	s.True(resp.Shows[0].ID < resp.Shows[1].ID, "tied shows must sort by id ASC")
}

func (s *ExploreServiceIntegrationSuite) TestGetUpcomingShows_HeadlinerAndVenueHydrated() {
	show, artist, venue := s.createShow("primary", 5)

	resp, err := s.exploreService.GetUpcomingShows(20, 0)
	s.Require().NoError(err)
	s.Require().Len(resp.Shows, 1)
	s.Equal(show.ID, resp.Shows[0].ID)
	s.Equal(artist.Name, resp.Shows[0].HeadlinerName)
	s.Equal(venue.Name, resp.Shows[0].VenueName)
	s.Equal("Phoenix", resp.Shows[0].VenueCity)
	s.Equal("AZ", resp.Shows[0].VenueState)
}

func (s *ExploreServiceIntegrationSuite) TestGetUpcomingShows_LimitClamped() {
	for i := 0; i < 5; i++ {
		s.createShow(fmt.Sprintf("show-%d", i), i+1)
	}

	// Limit 2 only returns 2 shows but total stays accurate.
	resp, err := s.exploreService.GetUpcomingShows(2, 0)
	s.Require().NoError(err)
	s.Len(resp.Shows, 2)
	s.Equal(int64(5), resp.Total)

	// Out-of-range limit gets clamped to maxUpcomingShowsLimit.
	resp, err = s.exploreService.GetUpcomingShows(9999, 0)
	s.Require().NoError(err)
	s.Equal(maxUpcomingShowsLimit, resp.Limit)
}

func (s *ExploreServiceIntegrationSuite) TestGetUpcomingShows_OffsetPaginates() {
	for i := 0; i < 5; i++ {
		s.createShow(fmt.Sprintf("show-%d", i), i+1)
	}

	page1, err := s.exploreService.GetUpcomingShows(2, 0)
	s.Require().NoError(err)
	s.Len(page1.Shows, 2)

	page2, err := s.exploreService.GetUpcomingShows(2, 2)
	s.Require().NoError(err)
	s.Len(page2.Shows, 2)

	// Page 2 IDs must be disjoint from page 1 IDs.
	page1IDs := map[uint]bool{page1.Shows[0].ID: true, page1.Shows[1].ID: true}
	for _, sh := range page2.Shows {
		s.False(page1IDs[sh.ID], "page 2 must not overlap page 1")
	}
}

// ──────────────────────────────────────────────
// GetFeatured
// ──────────────────────────────────────────────

func (s *ExploreServiceIntegrationSuite) TestGetFeatured_EmptyReturnsNullSlots() {
	resp, err := s.exploreService.GetFeatured()
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Nil(resp.Bill)
	s.Nil(resp.Collection)
}

func (s *ExploreServiceIntegrationSuite) TestGetFeatured_PopulatedBillAndCollection() {
	admin := s.createAdmin("admin1")
	show, artist, _ := s.createShow("bill-show", 14)
	coll := s.createCollection(admin.ID, "featured-coll", true)

	note := "Best bill of the month"
	_, err := s.featuredSlotService.SetActiveSlot("bill", show.ID, &note, admin.ID)
	s.Require().NoError(err)

	collNote := "Editor's pick"
	_, err = s.featuredSlotService.SetActiveSlot("collection", coll.ID, &collNote, admin.ID)
	s.Require().NoError(err)

	resp, err := s.exploreService.GetFeatured()
	s.Require().NoError(err)
	s.Require().NotNil(resp.Bill)
	s.Equal(show.ID, resp.Bill.ID)
	s.Equal(show.Title, resp.Bill.Title)
	s.Equal(artist.Name, resp.Bill.HeadlinerName)
	s.Equal("Best bill of the month", *resp.Bill.CuratorNote)
	s.NotEmpty(resp.Bill.CuratorNoteHTML)

	s.Require().NotNil(resp.Collection)
	s.Equal(coll.ID, resp.Collection.ID)
	s.Equal(coll.Title, resp.Collection.Title)
	s.Equal("Editor's pick", *resp.Collection.CuratorNote)
}

// TestGetFeatured_DeletedReferentCollapsesGracefully is the canonical
// defensive-read test the ticket calls out: a slot pointing at a row
// that no longer exists must return nil rather than 500.
func (s *ExploreServiceIntegrationSuite) TestGetFeatured_DeletedReferentCollapsesGracefully() {
	admin := s.createAdmin("admin2")
	show, _, _ := s.createShow("doomed", 14)

	note := "Will be deleted"
	_, err := s.featuredSlotService.SetActiveSlot("bill", show.ID, &note, admin.ID)
	s.Require().NoError(err)

	// Drop the show + its associations (FK cascade handles the
	// show_artists / show_venues rows).
	s.Require().NoError(s.db.Exec("DELETE FROM show_artists WHERE show_id = ?", show.ID).Error)
	s.Require().NoError(s.db.Exec("DELETE FROM show_venues WHERE show_id = ?", show.ID).Error)
	s.Require().NoError(s.db.Exec("DELETE FROM shows WHERE id = ?", show.ID).Error)

	resp, err := s.exploreService.GetFeatured()
	s.Require().NoError(err, "stale slot must NOT 500")
	s.Nil(resp.Bill, "deleted referent collapses to nil")
}

func (s *ExploreServiceIntegrationSuite) TestGetFeatured_PrivateCollectionExcluded() {
	admin := s.createAdmin("admin3")
	// Feature a public collection, then flip it private — the slot row
	// exists but /explore's consumer-side defensive read must hide it.
	// Write-side validation (PSY-850) rejects featuring an already-
	// private collection, so we set first then flip; the post-insert
	// status-cascade case is intentionally out of scope of write-side
	// validation per PSY-850 (status-change cascade follow-up deferred).
	coll := s.createCollection(admin.ID, "secret", true)

	_, err := s.featuredSlotService.SetActiveSlot("collection", coll.ID, nil, admin.ID)
	s.Require().NoError(err)

	s.Require().NoError(s.db.Model(coll).Update("is_public", false).Error)

	resp, err := s.exploreService.GetFeatured()
	s.Require().NoError(err)
	s.Nil(resp.Collection, "private collection must not surface on /explore")
}

func (s *ExploreServiceIntegrationSuite) TestGetFeatured_NonApprovedBillExcluded() {
	admin := s.createAdmin("admin4")
	// Feature an approved show, then flip status to pending — the slot
	// row exists but /explore's consumer-side defensive read collapses
	// the bill to nil. Mirrors the private-collection case: write-side
	// validation now blocks featuring a non-approved show up front, but
	// post-insert status flips still need consumer-side defense.
	show, _, _ := s.createShow("pending-shift", 14)

	_, err := s.featuredSlotService.SetActiveSlot("bill", show.ID, nil, admin.ID)
	s.Require().NoError(err)

	s.Require().NoError(s.db.Model(show).Update("status", catalogm.ShowStatusPending).Error)

	resp, err := s.exploreService.GetFeatured()
	s.Require().NoError(err)
	s.Nil(resp.Bill, "non-approved referent collapses to nil")
}

// ──────────────────────────────────────────────
// GetShuffleTarget
// ──────────────────────────────────────────────

func (s *ExploreServiceIntegrationSuite) TestGetShuffleTarget_EmptyPoolReturnsNil() {
	resp, err := s.exploreService.GetShuffleTarget()
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Nil(resp.ArtistID)
	s.Nil(resp.ArtistName)
}

// TestGetShuffleTarget_ReturnsFromQualifyingPool asserts membership in
// the ±90-day window. We seed three artists: one with a show 30 days
// ago, one with a show 30 days in the future, one with a show 200 days
// out. Repeated picks must come from the first two artists.
func (s *ExploreServiceIntegrationSuite) TestGetShuffleTarget_ReturnsFromQualifyingPool() {
	_, recentArtist, _ := s.createShow("recent", -30)    // within window (past)
	_, upcomingArtist, _ := s.createShow("upcoming", 30) // within window (future)
	_, farArtist, _ := s.createShow("far", 200)          // outside window

	qualifying := map[uint]bool{
		recentArtist.ID:   true,
		upcomingArtist.ID: true,
	}

	// 10 picks — strict pool membership.
	for i := 0; i < 10; i++ {
		resp, err := s.exploreService.GetShuffleTarget()
		s.Require().NoError(err)
		s.Require().NotNil(resp.ArtistID)
		s.True(qualifying[*resp.ArtistID],
			"shuffle picked artist %d, expected one of {%d, %d}; far-out artist %d should never appear",
			*resp.ArtistID, recentArtist.ID, upcomingArtist.ID, farArtist.ID)
	}
}

func (s *ExploreServiceIntegrationSuite) TestGetShuffleTarget_RespectsApprovedStatus() {
	// Insert a single artist whose only show is non-approved. Should
	// not appear in the shuffle pool.
	city := "Phoenix"
	state := "AZ"
	show := &catalogm.Show{
		Title:     "pending-only",
		EventDate: time.Now().UTC().AddDate(0, 0, 30),
		City:      &city,
		State:     &state,
		Status:    catalogm.ShowStatusPending,
	}
	s.Require().NoError(s.db.Create(show).Error)
	slug := "pending-artist"
	artist := &catalogm.Artist{Name: "Pending Artist", Slug: &slug}
	s.Require().NoError(s.db.Create(artist).Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO show_artists (show_id, artist_id, position, set_type)
	                                  VALUES (?, ?, 0, 'headliner')`, show.ID, artist.ID).Error)

	resp, err := s.exploreService.GetShuffleTarget()
	s.Require().NoError(err)
	s.Nil(resp.ArtistID, "artist with only non-approved shows must NOT be eligible")
}
