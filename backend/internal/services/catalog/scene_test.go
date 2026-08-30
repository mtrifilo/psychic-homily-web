package catalog

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/utils"
)

// seedMetro resolves a (city, state) to its CBSA code for test fixtures, mirroring
// the production write paths that set venues.metro / artists.metro via the
// geocoder (PSY-1255 step C). Returns nil for a non-US / no-CBSA place.
func seedMetro(city, state string) *string {
	return geo.MetroPointer(geo.Default(), city, state, usCountry)
}

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestBuildSceneSlug(t *testing.T) {
	tests := []struct {
		city, state, expected string
	}{
		{"Phoenix", "AZ", "phoenix-az"},
		{"New York", "NY", "new-york-ny"},
		{"San Francisco", "CA", "san-francisco-ca"},
		{"Mesa", "AZ", "mesa-az"},
		{"Winston-Salem", "NC", "winston-salem-nc"},
	}
	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, buildSceneSlug(tc.city, tc.state))
		})
	}
}

// PSY-1831. The collision the collapse exists for: a CBSA group and a fallback
// group whose literal city is that metro's principal city both slugify to
// "phoenix-az". The CBSA group wins because /scenes/phoenix-az resolves to it.
func TestCollapseSceneGroupsToCanonicalSlug(t *testing.T) {
	g := geo.Default()
	phoenixCBSA := *seedMetro("Phoenix", "AZ")
	metroGroup := sceneVenueGroup{Metro: phoenixCBSA, City: "Phoenix", State: "AZ", VenueCount: 2, ShowCount: 3}
	driftedGroup := sceneVenueGroup{City: "Phoenix", State: "AZ", VenueCount: 9, ShowCount: 99}
	tucsonGroup := sceneVenueGroup{Metro: *seedMetro("Tucson", "AZ"), City: "Tucson", State: "AZ", VenueCount: 4, ShowCount: 5}

	t.Run("uncontested slugs pass through untouched", func(t *testing.T) {
		in := []sceneVenueGroup{metroGroup, tucsonGroup}
		assert.Equal(t, in, collapseSceneGroupsToCanonicalSlug(in, g, "test"))
	})

	t.Run("the group the slug resolves to survives, regardless of input order", func(t *testing.T) {
		for _, in := range [][]sceneVenueGroup{
			{metroGroup, driftedGroup, tucsonGroup},
			{driftedGroup, metroGroup, tucsonGroup},
		} {
			out := collapseSceneGroupsToCanonicalSlug(in, g, "test")
			require.Len(t, out, 2)
			var phoenix sceneVenueGroup
			for _, grp := range out {
				if sceneGroupSlug(grp) == "phoenix-az" {
					phoenix = grp
				}
			}
			assert.Equal(t, metroGroup, phoenix,
				"the drifted group's larger counts must not win, and must not be summed in")
		}
	})

	t.Run("a fallback city with no CBSA is canonical for its own slug", func(t *testing.T) {
		// Neither group pins a CBSA, so neither is drift and neither collides.
		nonUS := []sceneVenueGroup{
			{City: "Montreal", State: "QC", VenueCount: 2, ShowCount: 3},
			{City: "Toronto", State: "ON", VenueCount: 2, ShowCount: 3},
		}
		assert.Equal(t, nonUS, collapseSceneGroupsToCanonicalSlug(nonUS, g, "test"))
	})

	t.Run("two no-CBSA groups collide on spelling, and the lowest literal wins", func(t *testing.T) {
		// sceneGroupKeySQL only lower/trims; buildSceneSlug also maps ' ' to '-'.
		// No metro drift is involved, and both groups match their slug scope, so
		// the CBSA test cannot separate them. ParseSceneSlug falls through to
		// ORDER BY city, state LIMIT 1, and ' ' sorts before '-'.
		spaced := sceneVenueGroup{City: "Saint Jerome", State: "QC", VenueCount: 2, ShowCount: 3}
		hyphenated := sceneVenueGroup{City: "Saint-Jerome", State: "QC", VenueCount: 9, ShowCount: 99}
		require.Equal(t, sceneGroupSlug(spaced), sceneGroupSlug(hyphenated), "fixture must actually collide")

		for _, in := range [][]sceneVenueGroup{{spaced, hyphenated}, {hyphenated, spaced}} {
			out := collapseSceneGroupsToCanonicalSlug(in, g, "test")
			require.Len(t, out, 1)
			assert.Equal(t, spaced, out[0],
				"the group the slug resolves to wins; the other's larger counts must not buy it the row")
		}
	})

	t.Run("a nil geocoder still collapses deterministically", func(t *testing.T) {
		// Without a geocoder no group can match its slug scope, so the tiebreak
		// carries the choice — it must still be one row, and the same one twice.
		in := []sceneVenueGroup{driftedGroup, metroGroup}
		first := collapseSceneGroupsToCanonicalSlug(in, nil, "test")
		require.Len(t, first, 1)
		assert.Equal(t, first, collapseSceneGroupsToCanonicalSlug([]sceneVenueGroup{metroGroup, driftedGroup}, nil, "test"))
	})
}

func TestSlugMissCache_ExpiresAndBounds(t *testing.T) {
	s := &SceneService{slugMisses: make(map[string]time.Time)}

	assert.False(t, s.slugMissCached("nope-xx"))
	s.rememberSlugMiss("Nope-XX")
	assert.True(t, s.slugMissCached("nope-xx"), "cache key is case-insensitive")

	s.missMu.Lock()
	s.slugMisses["nope-xx"] = time.Now().Add(-time.Second)
	s.missMu.Unlock()
	assert.False(t, s.slugMissCached("nope-xx"), "expired miss must not 404")

	// Overflow drops the map rather than growing without bound.
	s.slugMisses = make(map[string]time.Time)
	for i := 0; i < sceneSlugMissCacheMax; i++ {
		s.rememberSlugMiss(fmt.Sprintf("slug-%d-xx", i))
	}
	s.rememberSlugMiss("overflow-xx")
	s.missMu.Lock()
	n := len(s.slugMisses)
	_, keptOld := s.slugMisses["slug-0-xx"]
	_, hasNew := s.slugMisses["overflow-xx"]
	s.missMu.Unlock()
	assert.Equal(t, 1, n)
	assert.False(t, keptOld)
	assert.True(t, hasNew)
}

func TestSlugMissCache_NilMapIsNoop(t *testing.T) {
	s := &SceneService{}
	assert.False(t, s.slugMissCached("nope-xx"))
	s.rememberSlugMiss("nope-xx")
	assert.False(t, s.slugMissCached("nope-xx"))
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type SceneServiceIntegrationTestSuite struct {
	suite.Suite
	testDB       *testutil.TestDatabase
	db           *gorm.DB
	sceneService *SceneService
}

func (suite *SceneServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.sceneService = NewSceneService(suite.testDB.DB)
}

func (suite *SceneServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *SceneServiceIntegrationTestSuite) TearDownTest() {
	suite.sceneService.missMu.Lock()
	suite.sceneService.slugMisses = make(map[string]time.Time)
	suite.sceneService.missMu.Unlock()

	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM tag_aliases")
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM festival_artists")
	_, _ = sqlDB.Exec("DELETE FROM festival_venues")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	// Collections are cleared explicitly rather than left to the users delete
	// below. Both collections.creator_id and collection_items.added_by_user_id
	// are ON DELETE CASCADE, so the cascade would in fact reach them — but a
	// teardown that only works by cascade breaks the moment a collection is
	// seeded with no user, and this list is read as the inventory of what a
	// test may leave behind. Items go first so the pair reads in FK order like
	// every other block here (PSY-1847).
	_, _ = sqlDB.Exec("DELETE FROM collection_items")
	_, _ = sqlDB.Exec("DELETE FROM collections")
	// Label + relationship rows reference artists, so they MUST be cleared
	// first: this teardown ignores errors, so an FK-blocked "DELETE FROM
	// artists" fails silently and leaks the whole roster into the next test
	// (observed as inflated scene counts and duplicate-key creates once the
	// label-hub tests, PSY-1530, became the first here to seed these tables).
	// Votes go before relationships — that FK is ON DELETE NO ACTION.
	_, _ = sqlDB.Exec("DELETE FROM artist_relationship_votes")
	_, _ = sqlDB.Exec("DELETE FROM artist_relationships")
	_, _ = sqlDB.Exec("DELETE FROM artist_labels")
	_, _ = sqlDB.Exec("DELETE FROM release_labels")
	_, _ = sqlDB.Exec("DELETE FROM labels")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM scenes")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestSceneServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(SceneServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *SceneServiceIntegrationTestSuite) createVerifiedVenue(name, city, state string) *catalogm.Venue {
	venue := &catalogm.Venue{
		Name:     name,
		City:     city,
		State:    state,
		Metro:    seedMetro(city, state),
		Verified: true,
	}
	// Create as verified=true, then update to true (GORM bool gotcha: false is zero-value)
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	// Explicitly set Verified = true
	suite.db.Model(venue).Update("verified", true)
	return venue
}

// createVerifiedVenueNullMetro seeds a verified venue with a city/state that DOES
// pin a CBSA but no metro column — the venue-side twin of createArtistInNullMetro,
// and the drift ReconcileVenueMetros repairs (PSY-1831).
func (suite *SceneServiceIntegrationTestSuite) createVerifiedVenueNullMetro(name, city, state string) *catalogm.Venue {
	suite.Require().NotNil(seedMetro(city, state), "fixture is only meaningful for a city that pins a CBSA")
	venue := &catalogm.Venue{Name: name, City: city, State: state, Verified: true}
	suite.Require().NoError(suite.db.Create(venue).Error)
	suite.db.Model(venue).Update("verified", true)
	suite.Require().Nil(venue.Metro)
	return venue
}

func (suite *SceneServiceIntegrationTestSuite) createUnverifiedVenue(name, city, state string) *catalogm.Venue {
	venue := &catalogm.Venue{
		Name:  name,
		City:  city,
		State: state,
		Metro: seedMetro(city, state),
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	// Explicitly set Verified = false (GORM bool gotcha: default is true in DB)
	suite.db.Model(venue).Update("verified", false)
	return venue
}

// createArtist seeds an artist LOCAL to the suite's scene (Phoenix, AZ) so it
// counts toward the scene under the PSY-1233 home-city filter. Use createArtistIn
// for a touring act based elsewhere.
func (suite *SceneServiceIntegrationTestSuite) createArtist(name string) *catalogm.Artist {
	return suite.createArtistIn(name, "Phoenix", "AZ")
}

// createArtistIn seeds an artist with an explicit home city/state (+ its derived
// metro) — used to seed bands based elsewhere, who must NOT appear in this
// scene's roster under the metro-keyed model (PSY-1255 step C).
func (suite *SceneServiceIntegrationTestSuite) createArtistIn(name, city, state string) *catalogm.Artist {
	artist := &catalogm.Artist{Name: name, City: stringPtr(city), State: stringPtr(state), Metro: seedMetro(city, state)}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

// createArtistInNullMetro seeds an artist with home city/state but NO metro column —
// the PSY-1237 tail that roster matching must cover via CBSA member places.
func (suite *SceneServiceIntegrationTestSuite) createArtistInNullMetro(name, city, state string) *catalogm.Artist {
	artist := &catalogm.Artist{Name: name, City: stringPtr(city), State: stringPtr(state)}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	suite.Require().Nil(artist.Metro)
	return artist
}

// seedLabelMemberships creates a label and puts every artist on it. It is the
// membership primitive seedLabelWithRoster builds on: label hubs are derived
// from the artist_labels fact table, so a test that only needs memberships
// should not have to write the `shared_label` clique too.
func (suite *SceneServiceIntegrationTestSuite) seedLabelMemberships(
	label *catalogm.Label, artists []*catalogm.Artist,
) {
	suite.Require().NoError(suite.db.Create(label).Error)
	for _, a := range artists {
		suite.Require().NoError(suite.db.Create(&catalogm.ArtistLabel{
			ArtistID: a.ID, LabelID: label.ID,
		}).Error)
	}
}

func (suite *SceneServiceIntegrationTestSuite) createUser() *authm.User {
	user := &authm.User{
		Email:         stringPtr(fmt.Sprintf("scene-user-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *SceneServiceIntegrationTestSuite) createApprovedShow(title string, venueID, artistID, userID uint, eventDate time.Time) *catalogm.Show {
	show := &catalogm.Show{
		Title:       title,
		EventDate:   eventDate,
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)

	err = suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error
	suite.Require().NoError(err)

	err = suite.db.Create(&catalogm.ShowArtist{ShowID: show.ID, ArtistID: artistID, Position: 0}).Error
	suite.Require().NoError(err)

	return show
}

func (suite *SceneServiceIntegrationTestSuite) createFestival(name, city, state string) {
	festival := &catalogm.Festival{
		Name:        name,
		Slug:        fmt.Sprintf("%s-%d", name, time.Now().UnixNano()),
		SeriesSlug:  name,
		EditionYear: 2026,
		City:        stringPtr(city),
		State:       stringPtr(state),
		// Mirror the production write paths, which stamp the derived metro
		// alongside the location (PSY-1278) — the scene festival_count is
		// metro-keyed for metro scenes.
		Metro:     seedMetro(city, state),
		StartDate: "2026-03-01",
		EndDate:   "2026-03-03",
	}
	err := suite.db.Create(festival).Error
	suite.Require().NoError(err)
}

// seedSceneData creates data for Phoenix to qualify as a scene:
// 3 verified venues + 5 upcoming shows with artists.
func (suite *SceneServiceIntegrationTestSuite) seedSceneData() (venues []*catalogm.Venue, artists []*catalogm.Artist) {
	user := suite.createUser()

	v1 := suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")
	venues = []*catalogm.Venue{v1, v2, v3}

	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	a3 := suite.createArtist("Band C")
	artists = []*catalogm.Artist{a1, a2, a3}

	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("Show 1", v1.ID, a1.ID, user.ID, future)
	suite.createApprovedShow("Show 2", v1.ID, a2.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("Show 3", v2.ID, a1.ID, user.ID, future.AddDate(0, 0, 2))
	suite.createApprovedShow("Show 4", v2.ID, a3.ID, user.ID, future.AddDate(0, 0, 3))
	suite.createApprovedShow("Show 5", v3.ID, a2.ID, user.ID, future.AddDate(0, 0, 4))

	return venues, artists
}

// =============================================================================
// ListScenes Tests
// =============================================================================

func (suite *SceneServiceIntegrationTestSuite) TestListScenes_Empty() {
	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Empty(scenes)
}

func (suite *SceneServiceIntegrationTestSuite) TestListScenes_BelowThreshold_TooFewVenues() {
	// Only 1 verified venue — below the 2-verified-venue threshold
	user := suite.createUser()
	v := suite.createVerifiedVenue("Venue A", "Tucson", "AZ")
	a := suite.createArtistIn("Tucson Act", "Tucson", "AZ")
	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("Show 1", v.ID, a.ID, user.ID, future)
	suite.createApprovedShow("Show 2", v.ID, a.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("Show 3", v.ID, a.ID, user.ID, future.AddDate(0, 0, 2))

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Empty(scenes)
}

func (suite *SceneServiceIntegrationTestSuite) TestListScenes_BelowThreshold_TooFewShows() {
	// 2 verified venues but only 2 shows — below the 3-show threshold
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Venue X", "Flagstaff", "AZ")
	v2 := suite.createVerifiedVenue("Venue Y", "Flagstaff", "AZ")
	a := suite.createArtist("Flag Act")
	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("Show 1", v1.ID, a.ID, user.ID, future)
	suite.createApprovedShow("Show 2", v2.ID, a.ID, user.ID, future.AddDate(0, 0, 1))

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Empty(scenes)
}

func (suite *SceneServiceIntegrationTestSuite) TestListScenes_MeetsThreshold() {
	suite.seedSceneData()

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().Len(scenes, 1)

	scene := scenes[0]
	suite.Equal("Phoenix", scene.City)
	suite.Equal("AZ", scene.State)
	suite.Equal("phoenix-az", scene.Slug)
	suite.GreaterOrEqual(scene.VenueCount, 2)
	suite.GreaterOrEqual(scene.TotalShowCount, 3)
	suite.GreaterOrEqual(scene.UpcomingShowCount, 3)
}

// TestListScenes_ShowsThisWeek (PSY-1309): the ≤7-day slice counts only shows
// inside [now, now+7d). Dates are owned by this test and kept WELL clear of the
// window boundary — seedSceneData's first show sits at exactly now+7d, which
// races the service's own clock (its weekAhead is computed milliseconds later).
func (suite *SceneServiceIntegrationTestSuite) TestListScenes_ShowsThisWeek() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	a := suite.createArtist("Week Band")

	now := time.Now().UTC()
	// In-window: +2d and +3d. Out-of-window: +10/+11/+12d (they also carry the
	// scene past the 3-show listing threshold).
	suite.createApprovedShow("This Week 1", v1.ID, a.ID, user.ID, now.AddDate(0, 0, 2))
	suite.createApprovedShow("This Week 2", v2.ID, a.ID, user.ID, now.AddDate(0, 0, 3))
	suite.createApprovedShow("Later 1", v1.ID, a.ID, user.ID, now.AddDate(0, 0, 10))
	suite.createApprovedShow("Later 2", v2.ID, a.ID, user.ID, now.AddDate(0, 0, 11))
	suite.createApprovedShow("Later 3", v1.ID, a.ID, user.ID, now.AddDate(0, 0, 12))

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().Len(scenes, 1)
	suite.Equal(2, scenes[0].ShowsThisWeek, "only the two <7d shows count")
	suite.Equal(5, scenes[0].UpcomingShowCount, "next-7-days shows are also upcoming")
}

// The invariant PSY-1623 exists for: a count shown NEXT TO a link to
// /scenes/{slug}/week must be that page's own total. Asserting the two service
// calls against each other is what keeps them tied — a fixed expected number
// would let both drift together.
//
// Bounds come from sceneCalendarWeekWindow rather than from `now` plus an
// offset: the boundary cases (a Monday show, a Sunday-night show) are the ones
// that separate a calendar week from a rolling one, and they can only be
// addressed relative to the week's own edges.
func (suite *SceneServiceIntegrationTestSuite) TestListScenes_ShowsCalendarWeekMatchesWeekPage() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	a := suite.createArtist("Calendar Band")

	// No seeded venue carries a timezone, so the scene resolves its week in the
	// state map's zone — the same fallback the week page takes.
	loc := utils.EventLocation(nil, "AZ")
	start, end := sceneCalendarWeekWindow(time.Now(), loc)

	// Both edges of the week, inclusive-start and exclusive-end.
	suite.createApprovedShow("Monday night", v1.ID, a.ID, user.ID, start.Add(20*time.Hour))
	suite.createApprovedShow("Sunday night", v2.ID, a.ID, user.ID, end.Add(-2*time.Hour))
	// Just outside each edge — the shows a rolling window would smear in.
	suite.createApprovedShow("Last Sunday", v1.ID, a.ID, user.ID, start.Add(-2*time.Hour))
	suite.createApprovedShow("Next Monday", v2.ID, a.ID, user.ID, end.Add(2*time.Hour))
	suite.createApprovedShow("Next month", v1.ID, a.ID, user.ID, end.AddDate(0, 0, 20))

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().Len(scenes, 1)
	suite.Equal(2, scenes[0].ShowsCalendarWeek, "only the Monday and Sunday shows sit inside the calendar week")

	week, err := suite.sceneService.GetSceneWeek("Phoenix", "AZ", "")
	suite.Require().NoError(err)
	suite.Equal(week.ShowCount, scenes[0].ShowsCalendarWeek,
		"the list count IS the week page's total, or the link lies about where it goes")
}

// Every scene gets its OWN Monday. A single window applied across the list
// would be right for whichever zone it was derived in and wrong for the rest,
// which is the bug the shared /shows heading still carries by design.
//
// The New York show is placed one hour into New York's Monday — before Phoenix's
// Monday has started, and before UTC's has finished. It counts for New York and
// for nobody else, so a global window fails this test in either direction.
func (suite *SceneServiceIntegrationTestSuite) TestListScenes_ShowsCalendarWeekIsPerSceneTimezone() {
	user := suite.createUser()
	phx1 := suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	phx2 := suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	ny1 := suite.createVerifiedVenue("Bowery Ballroom", "New York", "NY")
	ny2 := suite.createVerifiedVenue("Union Pool", "New York", "NY")
	phxBand := suite.createArtist("Desert Band")
	nyBand := suite.createArtistIn("Borough Band", "New York", "NY")

	now := time.Now()
	phxStart, _ := sceneCalendarWeekWindow(now, utils.EventLocation(nil, "AZ"))
	nyStart, _ := sceneCalendarWeekWindow(now, utils.EventLocation(nil, "NY"))
	// For the ~3 hours each Monday between New York's midnight and Phoenix's,
	// the two zones are in DIFFERENT ISO weeks, so their starts sit a week apart
	// rather than three hours and the boundary gap this test aims at does not
	// exist. Skipping is the honest response; asserting would fail CI for three
	// hours every week, and pinning the clock would mean threading one through
	// ListScenes for this test alone. The sibling tests below carry the
	// list-equals-page invariant in that window.
	if !nyStart.Before(phxStart) {
		suite.T().Skipf(
			"New York (%s) has rolled into the next ISO week ahead of Phoenix (%s); no boundary gap to test",
			nyStart.Format(time.RFC3339), phxStart.Format(time.RFC3339))
	}

	// Three shows per scene clears the listing threshold; the counted ones sit
	// one hour into each scene's own Monday.
	suite.createApprovedShow("PHX Monday", phx1.ID, phxBand.ID, user.ID, phxStart.Add(time.Hour))
	// The mirror of the New York case, an hour BEFORE Phoenix's Monday: already
	// Monday in UTC, still last week in Phoenix. Between them the two shows rule
	// out every fixed zone, not just the western ones.
	suite.createApprovedShow("PHX last Sunday", phx2.ID, phxBand.ID, user.ID, phxStart.Add(-time.Hour))
	suite.createApprovedShow("PHX later 1", phx2.ID, phxBand.ID, user.ID, phxStart.AddDate(0, 0, 30))
	suite.createApprovedShow("PHX later 2", phx1.ID, phxBand.ID, user.ID, phxStart.AddDate(0, 0, 31))
	suite.createApprovedShow("NY Monday", ny1.ID, nyBand.ID, user.ID, nyStart.Add(time.Hour))
	suite.createApprovedShow("NY later 1", ny2.ID, nyBand.ID, user.ID, nyStart.AddDate(0, 0, 30))
	suite.createApprovedShow("NY later 2", ny1.ID, nyBand.ID, user.ID, nyStart.AddDate(0, 0, 31))

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)

	// Keyed by state, not by slug: a metro scene lists under its CBSA's
	// PRINCIPAL city, which is "New York City" rather than the "New York" the
	// venues were seeded with. Asserting on a guessed slug would test the geo
	// dataset's naming instead of the count.
	byState := make(map[string]*contracts.SceneListResponse, len(scenes))
	for _, sc := range scenes {
		byState[sc.State] = sc
	}
	suite.Require().Contains(byState, "AZ")
	suite.Require().Contains(byState, "NY")
	suite.Equal(1, byState["AZ"].ShowsCalendarWeek, "the Phoenix show sits in Phoenix's week")
	suite.Equal(1, byState["NY"].ShowsCalendarWeek,
		"the New York show sits in New York's week, which opened three hours earlier")

	for _, sc := range scenes {
		week, wErr := suite.sceneService.GetSceneWeek(sc.City, sc.State, "")
		suite.Require().NoError(wErr)
		suite.Equal(week.ShowCount, sc.ShowsCalendarWeek, "%s must agree with its own week page", sc.Slug)
	}
}

// A quiet scene reports 0 rather than dropping out of the map, because the
// caller mutes a week link on exactly that value — a scene missing from the
// count query must not be indistinguishable from a scene with shows.
func (suite *SceneServiceIntegrationTestSuite) TestListScenes_ShowsCalendarWeekIsZeroForAQuietWeek() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	a := suite.createArtist("Quiet Band")

	_, end := sceneCalendarWeekWindow(time.Now(), utils.EventLocation(nil, "AZ"))
	suite.createApprovedShow("Later 1", v1.ID, a.ID, user.ID, end.AddDate(0, 0, 10))
	suite.createApprovedShow("Later 2", v2.ID, a.ID, user.ID, end.AddDate(0, 0, 11))
	suite.createApprovedShow("Later 3", v1.ID, a.ID, user.ID, end.AddDate(0, 0, 12))

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().Len(scenes, 1)
	suite.Equal(0, scenes[0].ShowsCalendarWeek)

	week, err := suite.sceneService.GetSceneWeek("Phoenix", "AZ", "")
	suite.Require().NoError(err)
	suite.Equal(week.ShowCount, scenes[0].ShowsCalendarWeek)
}

// TestGetSceneUpcomingShows (PSY-1309): soonest-first within the window,
// limit-capped, metro-scoped (a member-city show counts), and windowed (a
// beyond-window show doesn't).
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneUpcomingShows() {
	venues, artists := suite.seedSceneData() // Phoenix; its 5 shows sit at 7–11d (outside a 7d window)
	user := suite.createUser()
	tempe := suite.createVerifiedVenue("Yucca Tap Room", "Tempe", "AZ") // Phoenix-CBSA member city

	now := time.Now().UTC()
	day3 := suite.createApprovedShow("Day 3 Show", venues[0].ID, artists[0].ID, user.ID, now.AddDate(0, 0, 3))
	suite.createApprovedShow("Day 1 Tempe Show", tempe.ID, artists[1].ID, user.ID, now.AddDate(0, 0, 1))
	suite.createApprovedShow("Day 5 Show", venues[1].ID, artists[2].ID, user.ID, now.AddDate(0, 0, 5))

	// PSY-1325: more artists on the Day 3 bill — the summary must carry the
	// bill in position order (most shows have empty titles, so these names
	// ARE the display name). "Day 3 Same Slot" shares position 0 with the
	// headliner: created later → higher id → sorts AFTER it, pinning the
	// artists.id tie-break (same-position entries otherwise come back in
	// planner order, which can flip between runs).
	sameSlot := suite.createArtist("Day 3 Same Slot")
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{
		ShowID: day3.ID, ArtistID: sameSlot.ID, Position: 0,
	}).Error)
	opener := suite.createArtist("Day 3 Opener")
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{
		ShowID: day3.ID, ArtistID: opener.ID, Position: 1,
	}).Error)
	// artists[2] has a LOWER id than every artist above but the HIGHEST
	// position — under an id-only sort it would land second, so this row is
	// what proves position outranks id (the bill isn't accidentally id-ordered).
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{
		ShowID: day3.ID, ArtistID: artists[2].ID, Position: 2,
	}).Error)

	shows, err := suite.sceneService.GetSceneUpcomingShows("Phoenix", "AZ", 7, 3)
	suite.Require().NoError(err)
	suite.Require().Len(shows, 3)
	// Soonest first — and the Tempe (member-city) show is included AND first.
	suite.Equal("Day 1 Tempe Show", shows[0].Title)
	suite.Equal("Yucca Tap Room", shows[0].VenueName)
	suite.Equal([]string{artists[1].Name}, shows[0].ArtistNames)
	suite.Equal("Day 3 Show", shows[1].Title)
	// Bill in position order, id tie-break within a position: headliner
	// (pos 0, lower id) → same-slot (pos 0, higher id) → opener (pos 1) →
	// artists[2] (pos 2, LOWEST id — proves position outranks id).
	suite.Equal(
		[]string{artists[0].Name, "Day 3 Same Slot", "Day 3 Opener", artists[2].Name},
		shows[1].ArtistNames,
	)
	suite.Equal("Day 5 Show", shows[2].Title)

	// Limit caps the list (the 7–11d seed shows would qualify in a 30d window).
	capped, err := suite.sceneService.GetSceneUpcomingShows("Phoenix", "AZ", 30, 2)
	suite.Require().NoError(err)
	suite.Len(capped, 2)

	// Unknown scene → typed not-found.
	_, err = suite.sceneService.GetSceneUpcomingShows("Nowhere", "ZZ", 7, 3)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "scene not found")
}

// TestGetSceneUpcomingShows_ArtistsCarrySlugs (PSY-1846): the summary carries
// slug+name pairs so a bill can render as linked entities, in the SAME order as
// the legacy names, and an unslugged band survives with Slug "" rather than
// being dropped.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneUpcomingShows_ArtistsCarrySlugs() {
	venues, _ := suite.seedSceneData()
	user := suite.createUser()
	now := time.Now().UTC()

	// createArtist leaves slug NULL, which is the unslugged case verbatim:
	// artists.slug is nullable and GenerateSlug can return "" (PSY-1754).
	headliner := suite.createArtist("Slugged Headliner")
	suite.Require().NoError(suite.db.Model(headliner).Update("slug", "slugged-headliner").Error)
	unslugged := suite.createArtist("Unslugged Opener")
	suite.Require().Nil(unslugged.Slug)

	show := suite.createApprovedShow("Bill Link Show", venues[0].ID, headliner.ID, user.ID, now.AddDate(0, 0, 2))
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{
		ShowID: show.ID, ArtistID: unslugged.ID, Position: 1,
	}).Error)

	shows, err := suite.sceneService.GetSceneUpcomingShows("Phoenix", "AZ", 7, 10)
	suite.Require().NoError(err)
	// By title, not by index: seedSceneData's own shows sit on the window edge,
	// so which of them also come back is a timing detail this test doesn't own.
	row := suite.findSceneShow(shows, "Bill Link Show")

	suite.Equal([]contracts.SceneShowArtist{
		{Name: "Slugged Headliner", Slug: "slugged-headliner"},
		// Present, with an EMPTY slug — the frontend renders this one unlinked.
		// A NULL slug must never arrive as the string "<nil>" or drop the band.
		{Name: "Unslugged Opener", Slug: ""},
	}, row.Artists)

	// The legacy names field keeps working, and agrees with the pairs above.
	suite.Equal([]string{"Slugged Headliner", "Unslugged Opener"}, row.ArtistNames)
}

// TestSceneSurfaces_AllCarryArtistPairs (PSY-1846): the day, week and upcoming
// surfaces all render the linked-bill row, so none of them may serve a bill as
// names-only. They share one builder today; this is the assertion that fails if
// a surface is ever given its own mapping that forgets the pairs.
func (suite *SceneServiceIntegrationTestSuite) TestSceneSurfaces_AllCarryArtistPairs() {
	venues, artists := suite.seedSceneData()
	user := suite.createUser()

	// Anchor every surface on ONE seeded show at a known instant, so the day and
	// week keys are computed rather than guessed. seedSceneData's own shows sit
	// 7–11 days out, which lands them in an ISO week that depends on the weekday
	// the suite happens to run — not something this test should be sensitive to.
	anchor := time.Now().UTC().AddDate(0, 0, 3)
	suite.createApprovedShow("Anchor Show", venues[0].ID, artists[0].ID, user.ID, anchor)
	// Derive the week key in the SCENE's timezone, from the service's own
	// resolver, for the same reason the day key below is read back from the
	// week's buckets: GetSceneWeek buckets scene-locally, so a UTC-formatted key
	// names the wrong week whenever the anchor instant has already crossed into
	// Monday UTC but is still Sunday in the scene (Phoenix is UTC-7 year round).
	sceneLoc := suite.sceneService.sceneLocation(suite.sceneService.scopeFor("Phoenix", "AZ"), "AZ")
	anchorWeekKey := ISOWeekKey(anchor.In(sceneLoc))

	assertPaired := func(surface string, shows []contracts.SceneShowSummary) {
		// Guard against a vacuous pass: a surface that returns nothing, or bills
		// that are all empty, would satisfy the loop below trivially.
		billed := 0
		for _, s := range shows {
			if len(s.ArtistNames) == 0 {
				continue
			}
			billed++
			suite.Require().Len(s.Artists, len(s.ArtistNames),
				"%s: show %d serves a bill with no slug pairs", surface, s.ID)
			for i, name := range s.ArtistNames {
				suite.Equal(name, s.Artists[i].Name, "%s: show %d bill order diverges", surface, s.ID)
			}
		}
		suite.Require().Positive(billed, "%s returned no billed shows — assertion would be vacuous", surface)
	}

	upcoming, err := suite.sceneService.GetSceneUpcomingShows("Phoenix", "AZ", 30, 50)
	suite.Require().NoError(err)
	assertPaired("upcoming", upcoming)

	week, err := suite.sceneService.GetSceneWeek("Phoenix", "AZ", anchorWeekKey)
	suite.Require().NoError(err)
	weekShows := []contracts.SceneShowSummary{}
	for _, d := range week.Days {
		weekShows = append(weekShows, d.Shows...)
	}
	assertPaired("week", weekShows)

	// Take the day key from the week's own buckets rather than formatting the
	// anchor here: those keys are scene-LOCAL, and a UTC-formatted date lands on
	// the wrong day for any evening show west of Greenwich.
	anchorDay := ""
	for _, d := range week.Days {
		for _, s := range d.Shows {
			if s.Title == "Anchor Show" {
				anchorDay = d.Date
			}
		}
	}
	suite.Require().NotEmpty(anchorDay, "anchor show missing from its own week")

	day, err := suite.sceneService.GetSceneDay("Phoenix", "AZ", anchorDay)
	suite.Require().NoError(err)
	assertPaired("day", day.Shows)
}

// findSceneShow returns the summary with the given title, failing the test when
// it is absent.
func (suite *SceneServiceIntegrationTestSuite) findSceneShow(
	shows []contracts.SceneShowSummary, title string,
) contracts.SceneShowSummary {
	suite.T().Helper()
	for _, s := range shows {
		if s.Title == title {
			return s
		}
	}
	suite.Require().FailNowf("show not found", "no show titled %q in %+v", title, shows)
	return contracts.SceneShowSummary{}
}

// TestGetSceneUpcomingShows_NoBillOmitsBothArtistFields (PSY-1846): a show with
// no artists leaves BOTH artist fields empty, so `omitempty` keeps them off the
// wire exactly as artist_names alone did before the pairs were added. Asserted
// on the marshalled JSON, since that absence — not Go-side nil-ness — is the
// contract clients see (encoding/json drops a nil and an empty slice alike).
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneUpcomingShows_NoBillOmitsBothArtistFields() {
	venues, _ := suite.seedSceneData()
	user := suite.createUser()
	now := time.Now().UTC()

	// createApprovedShow always bills one artist, so strip it back off to get
	// the genuinely empty bill (a venue-only listing).
	show := suite.createApprovedShow("Open Decks", venues[0].ID, suite.createArtist("Removed").ID, user.ID, now.AddDate(0, 0, 2))
	suite.Require().NoError(suite.db.Where("show_id = ?", show.ID).Delete(&catalogm.ShowArtist{}).Error)

	shows, err := suite.sceneService.GetSceneUpcomingShows("Phoenix", "AZ", 7, 10)
	suite.Require().NoError(err)
	row := suite.findSceneShow(shows, "Open Decks")
	suite.Empty(row.ArtistNames)
	suite.Empty(row.Artists)

	encoded, err := json.Marshal(row)
	suite.Require().NoError(err)
	suite.NotContains(string(encoded), `"artist_names"`)
	suite.NotContains(string(encoded), `"artists"`)
}

// The weekly city page publishes each show as schema.org MusicEvent, which
// needs a real instant, a postal address and a price — none of which can be
// recovered from the scene-local EventDate string.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneShowsInRange_CarriesVenueDetail() {
	user := suite.createUser()
	crescent := suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	suite.Require().NoError(suite.db.Model(crescent).Updates(map[string]any{
		"slug":     "crescent-ballroom",
		"address":  "308 N 2nd Ave",
		"country":  "US",
		"timezone": "America/Phoenix",
	}).Error)
	suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ") // meets the 2-venue scene threshold
	artist := suite.createArtist("Riff Wood")

	// 20:00 Phoenix on the 27th. The date-only EventDate a client sees is
	// "2026-07-27"; re-parsing that as an instant lands on the 26th in Arizona,
	// which is exactly why StartsAt has to be carried separately.
	start := time.Date(2026, 7, 28, 3, 0, 0, 0, time.UTC)
	show := suite.createApprovedShow("", crescent.ID, artist.ID, user.ID, start)
	price := 20.0
	suite.Require().NoError(suite.db.Model(show).Updates(map[string]any{
		"slug":  "riff-wood-crescent-ballroom",
		"price": price,
	}).Error)

	phx, err := time.LoadLocation("America/Phoenix")
	suite.Require().NoError(err)
	shows, err := suite.sceneService.GetSceneShowsInRange(
		"Phoenix", "AZ", start.AddDate(0, 0, -1), start.AddDate(0, 0, 1), phx, 10)
	suite.Require().NoError(err)
	suite.Require().Len(shows, 1)

	got := shows[0]
	suite.Equal("2026-07-27", got.EventDate, "scene-local calendar date")
	suite.True(got.StartsAt.Equal(start), "absolute instant, not the calendar date")
	suite.Require().NotNil(got.Price)
	suite.InDelta(price, *got.Price, 0.001)
	suite.Equal("Crescent Ballroom", got.VenueName)
	suite.Equal("crescent-ballroom", got.VenueSlug)
	suite.Equal("308 N 2nd Ave", got.VenueAddress)
	suite.Equal("Phoenix", got.VenueCity)
	suite.Equal("AZ", got.VenueState)
	// Scenes are not US-only, so the country travels rather than being assumed
	// by whoever renders the address.
	suite.Equal("US", got.VenueCountry)
	suite.Equal("America/Phoenix", got.VenueTimezone)
}

// The street address of an UNVERIFIED venue is withheld everywhere else
// (buildVenueResponse, the show detail payload); a listing endpoint that leaked
// it would publish a house venue's address before any human reviewed it.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneShowsInRange_WithholdsUnverifiedAddress() {
	user := suite.createUser()
	suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	house := suite.createUnverifiedVenue("Someone's Basement", "Phoenix", "AZ")
	suite.Require().NoError(suite.db.Model(house).Update("address", "123 Private St").Error)
	artist := suite.createArtist("Basement Act")

	start := time.Now().UTC().AddDate(0, 0, 2)
	suite.createApprovedShow("House Show", house.ID, artist.ID, user.ID, start)

	shows, err := suite.sceneService.GetSceneShowsInRange(
		"Phoenix", "AZ", start.AddDate(0, 0, -1), start.AddDate(0, 0, 1), time.UTC, 10)
	suite.Require().NoError(err)
	suite.Require().Len(shows, 1)
	suite.Equal("Someone's Basement", shows[0].VenueName, "the venue is still listed")
	suite.Empty(shows[0].VenueAddress, "but its street address is not published")
	suite.Equal("Phoenix", shows[0].VenueCity, "city-level location stays")
}

// Venue columns must all describe ONE room. Under the previous MIN(name) +
// GROUP BY shape a multi-venue show would have paired the alphabetically-first
// venue's NAME with an arbitrary sibling's address.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneShowsInRange_MultiVenueStaysCoherent() {
	user := suite.createUser()
	alpha := suite.createVerifiedVenue("Alpha Room", "Phoenix", "AZ")
	suite.Require().NoError(suite.db.Model(alpha).Update("address", "1 Alpha Way").Error)
	omega := suite.createVerifiedVenue("Omega Room", "Phoenix", "AZ")
	suite.Require().NoError(suite.db.Model(omega).Update("address", "99 Omega Way").Error)
	artist := suite.createArtist("Two Room Band")

	start := time.Now().UTC().AddDate(0, 0, 2)
	show := suite.createApprovedShow("Two Venue Show", omega.ID, artist.ID, user.ID, start)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: alpha.ID}).Error)

	shows, err := suite.sceneService.GetSceneShowsInRange(
		"Phoenix", "AZ", start.AddDate(0, 0, -1), start.AddDate(0, 0, 1), time.UTC, 10)
	suite.Require().NoError(err)
	suite.Require().Len(shows, 1)
	suite.Equal("Alpha Room", shows[0].VenueName, "lowest name wins, as MIN(name) did")
	suite.Equal("1 Alpha Way", shows[0].VenueAddress, "and the address is THAT room's")
}

func (suite *SceneServiceIntegrationTestSuite) TestListScenes_IncludesGeocodedCoords() {
	// A qualifying scene gets its coordinate from the geocoded (city, state)
	// centroid — the same offline geocoder GetShowCities and venue writes use.
	// No venue coordinates are involved.
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	a := suite.createArtist("PHX Act")
	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("S1", v1.ID, a.ID, user.ID, future)
	suite.createApprovedShow("S2", v2.ID, a.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("S3", v1.ID, a.ID, user.ID, future.AddDate(0, 0, 2))

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().Len(scenes, 1)

	scene := scenes[0]
	suite.Require().NotNil(scene.Latitude)
	suite.Require().NotNil(scene.Longitude)
	// Phoenix, AZ ≈ (33.45, -112.07).
	suite.InDelta(33.45, *scene.Latitude, 1.0)
	suite.InDelta(-112.07, *scene.Longitude, 1.0)
}

func (suite *SceneServiceIntegrationTestSuite) TestListScenes_NullCoordsWhenCityUnknown() {
	// A city the geocoder can't resolve → coords stay nil (null-safe: the scene
	// still lists, it just can't be placed on the map).
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Hall A", "Faketown", "ZZ")
	v2 := suite.createVerifiedVenue("Hall B", "Faketown", "ZZ")
	a := suite.createArtist("Fake Act")
	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("S1", v1.ID, a.ID, user.ID, future)
	suite.createApprovedShow("S2", v2.ID, a.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("S3", v1.ID, a.ID, user.ID, future.AddDate(0, 0, 2))

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().Len(scenes, 1)

	scene := scenes[0]
	suite.Nil(scene.Latitude)
	suite.Nil(scene.Longitude)
}

func (suite *SceneServiceIntegrationTestSuite) TestListScenes_QualifiesWithPastShowsOnly() {
	// A city with 2 verified venues and 3 past shows (no upcoming) should still qualify
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("The Rialto", "Tucson", "AZ")
	v2 := suite.createVerifiedVenue("Club Congress", "Tucson", "AZ")
	a := suite.createArtistIn("Tucson Band", "Tucson", "AZ")

	past := time.Now().UTC().AddDate(0, 0, -30)
	suite.createApprovedShow("Past Tucson Show 1", v1.ID, a.ID, user.ID, past)
	suite.createApprovedShow("Past Tucson Show 2", v2.ID, a.ID, user.ID, past.AddDate(0, 0, -7))
	suite.createApprovedShow("Past Tucson Show 3", v1.ID, a.ID, user.ID, past.AddDate(0, 0, -14))

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().Len(scenes, 1)

	scene := scenes[0]
	suite.Equal("Tucson", scene.City)
	suite.Equal("AZ", scene.State)
	suite.Equal(3, scene.TotalShowCount)
	suite.Equal(0, scene.UpcomingShowCount)
}

func (suite *SceneServiceIntegrationTestSuite) TestListScenes_MeetsMinimumThreshold() {
	// A city with exactly 2 venues and 3 shows should qualify
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("The Mint", "Los Angeles", "CA")
	v2 := suite.createVerifiedVenue("The Echo", "Los Angeles", "CA")
	a := suite.createArtistIn("LA Band", "Los Angeles", "CA")

	future := time.Now().UTC().AddDate(0, 0, 14)
	suite.createApprovedShow("LA Show 1", v1.ID, a.ID, user.ID, future)
	suite.createApprovedShow("LA Show 2", v2.ID, a.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("LA Show 3", v1.ID, a.ID, user.ID, future.AddDate(0, 0, 2))

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().Len(scenes, 1)

	scene := scenes[0]
	suite.Equal("Los Angeles", scene.City)
	suite.Equal(2, scene.VenueCount)
	suite.Equal(3, scene.TotalShowCount)
	suite.Equal(3, scene.UpcomingShowCount)
}

func (suite *SceneServiceIntegrationTestSuite) TestListScenes_MultipleScenes() {
	// Phoenix scene
	suite.seedSceneData()

	// Chicago scene
	user := suite.createUser()
	cv1 := suite.createVerifiedVenue("Metro", "Chicago", "IL")
	cv2 := suite.createVerifiedVenue("Empty Bottle", "Chicago", "IL")
	cv3 := suite.createVerifiedVenue("Thalia Hall", "Chicago", "IL")
	ca := suite.createArtistIn("Chicago Band", "Chicago", "IL")

	future := time.Now().UTC().AddDate(0, 0, 7)
	for i := 0; i < 7; i++ {
		venues := []*catalogm.Venue{cv1, cv2, cv3}
		suite.createApprovedShow(
			fmt.Sprintf("Chi Show %d", i),
			venues[i%3].ID, ca.ID, user.ID,
			future.AddDate(0, 0, i),
		)
	}

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().Len(scenes, 2)

	// Should be sorted by total show count descending
	// Chicago has 7, Phoenix has 5
	suite.Equal("Chicago", scenes[0].City)
	suite.Equal("Phoenix", scenes[1].City)
}

// TestListScenes_DriftedVenueMetroDoesNotSplitTheScene (PSY-1831): verified
// Phoenix rooms whose denormalized venues.metro was never written form their own
// fallback group, and its display identity slugifies to the SAME "phoenix-az" as
// the CBSA group's. Before the collapse the list published both, so /shows
// rendered two identical rows under one React key.
//
// The surviving row is the CBSA group — the scope /scenes/phoenix-az resolves to
// — and it carries that group's counts alone, unsummed, because the detail page
// selects its rooms by venues.metro and never shows the drifted ones.
func (suite *SceneServiceIntegrationTestSuite) TestListScenes_DriftedVenueMetroDoesNotSplitTheScene() {
	user := suite.createUser()
	metroA := suite.createVerifiedVenue("Metro Room A", "Phoenix", "AZ")
	metroB := suite.createVerifiedVenue("Metro Room B", "Phoenix", "AZ")
	suite.Require().NotNil(metroA.Metro, "the fixture must carry the CBSA the drifted rooms are missing")

	driftedA := suite.createVerifiedVenueNullMetro("Drifted Room A", "Phoenix", "AZ")
	driftedB := suite.createVerifiedVenueNullMetro("Drifted Room B", "Phoenix", "AZ")

	band := suite.createArtist("Local Band")
	future := time.Now().UTC().AddDate(0, 0, 3)
	// Both halves independently clear sceneMinVenues/sceneMinShows, so both
	// groups survive the HAVING and the collision is the list's to resolve.
	suite.createApprovedShow("Metro 1", metroA.ID, band.ID, user.ID, future)
	suite.createApprovedShow("Metro 2", metroB.ID, band.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("Metro 3", metroA.ID, band.ID, user.ID, future.AddDate(0, 0, 2))
	suite.createApprovedShow("Drift 1", driftedA.ID, band.ID, user.ID, future)
	suite.createApprovedShow("Drift 2", driftedB.ID, band.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("Drift 3", driftedA.ID, band.ID, user.ID, future.AddDate(0, 0, 2))

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)

	slugs := map[string]int{}
	for _, sc := range scenes {
		slugs[sc.Slug]++
	}
	suite.Equal(1, slugs["phoenix-az"], "one row per scene slug, whatever venues.metro says")
	suite.Require().Len(scenes, 1)
	suite.Equal("Phoenix", scenes[0].City)
	suite.Equal("AZ", scenes[0].State)
	suite.Equal(2, scenes[0].VenueCount, "the CBSA group's rooms only — the drifted pair is not summed in")
	suite.Equal(3, scenes[0].TotalShowCount, "counts match what /scenes/phoenix-az will print")

	// The number the surviving row publishes is the number its destination page
	// serves: the collapse must not leave the directory contradicting the page.
	count, err := suite.sceneService.verifiedVenueCount(suite.sceneService.scopeFor("Phoenix", "AZ"))
	suite.Require().NoError(err)
	suite.Equal(int64(scenes[0].VenueCount), count)
}

// TestListScenes_SpellingVariantsDoNotSplitTheScene (PSY-1831): the collision
// that needs no drift at all. sceneGroupKeySQL only lower/trims, while
// buildSceneSlug also maps spaces to dashes, so two spellings of one non-US city
// are two venue groups under one slug.
//
// The assertion is the CORRESPONDENCE, not a guess at which spelling wins:
// whichever literal ParseSceneSlug resolves the slug to is the one the list must
// publish, since that is the pair venuePredicate will serve. Asserting the
// agreement rather than a hardcoded winner also keeps the test honest about
// collation — Go compares the group minima byte-wise and Postgres orders under
// the database's collation, and this fails loudly if those ever disagree.
func (suite *SceneServiceIntegrationTestSuite) TestListScenes_SpellingVariantsDoNotSplitTheScene() {
	user := suite.createUser()
	spacedA := suite.createVerifiedVenue("Spaced A", "Saint Jerome", "QC")
	spacedB := suite.createVerifiedVenue("Spaced B", "Saint Jerome", "QC")
	hyphenA := suite.createVerifiedVenue("Hyphen A", "Saint-Jerome", "QC")
	hyphenB := suite.createVerifiedVenue("Hyphen B", "Saint-Jerome", "QC")
	suite.Require().Nil(spacedA.Metro, "a non-US city must not pin a CBSA — this collision is not metro drift")

	band := suite.createArtist("Local Band")
	future := time.Now().UTC().AddDate(0, 0, 3)
	suite.createApprovedShow("Sp 1", spacedA.ID, band.ID, user.ID, future)
	suite.createApprovedShow("Sp 2", spacedB.ID, band.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("Sp 3", spacedA.ID, band.ID, user.ID, future.AddDate(0, 0, 2))
	suite.createApprovedShow("Hy 1", hyphenA.ID, band.ID, user.ID, future)
	suite.createApprovedShow("Hy 2", hyphenB.ID, band.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("Hy 3", hyphenA.ID, band.ID, user.ID, future.AddDate(0, 0, 2))

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().Len(scenes, 1, "one row per scene slug, whatever the spelling variance")
	suite.Equal("saint-jerome-qc", scenes[0].Slug)

	resolvedCity, resolvedState, err := suite.sceneService.ParseSceneSlug("saint-jerome-qc")
	suite.Require().NoError(err)
	suite.Equal(resolvedCity, scenes[0].City, "the listed row must name the city its page will serve")
	suite.Equal(resolvedState, scenes[0].State)

	// And the counts must be that group's alone, not the pair summed.
	count, err := suite.sceneService.verifiedVenueCount(suite.sceneService.scopeFor(resolvedCity, resolvedState))
	suite.Require().NoError(err)
	suite.Equal(int64(scenes[0].VenueCount), count)
	suite.Equal(2, scenes[0].VenueCount)
}

// TestListScenes_MetroRollup is the headline step-C behavior: two cities sharing
// one CBSA (Minneapolis + Saint Paul → 33460) roll up to ONE scene displayed
// under the principal city, and the roster is every band BASED in the metro —
// including a suburb band that never played a local show — while a touring act
// based in another metro is excluded even though it played here.
func (suite *SceneServiceIntegrationTestSuite) TestListScenes_MetroRollup() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("First Avenue", "Minneapolis", "MN")
	v2 := suite.createVerifiedVenue("Turf Club", "Saint Paul", "MN")

	mpls := suite.createArtistIn("Minneapolis Band", "Minneapolis", "MN")
	suite.createArtistIn("Bloomington Band", "Bloomington", "MN") // suburb of the same metro; never plays locally
	tourer := suite.createArtistIn("Chicago Tourer", "Chicago", "IL")

	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("TC 1", v1.ID, mpls.ID, user.ID, future)
	suite.createApprovedShow("TC 2", v2.ID, mpls.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("TC 3", v1.ID, tourer.ID, user.ID, future.AddDate(0, 0, 2)) // tourer plays here

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().Len(scenes, 1)
	suite.Equal("Minneapolis", scenes[0].City) // principal city of CBSA 33460
	suite.Equal("MN", scenes[0].State)
	suite.Equal("minneapolis-mn", scenes[0].Slug)
	suite.Equal(2, scenes[0].VenueCount) // both cities rolled up

	roster, total, err := suite.sceneService.GetActiveArtists("Minneapolis", "MN", 180, 50, 0)
	suite.Require().NoError(err)
	names := map[string]bool{}
	for _, a := range roster {
		names[a.Name] = true
	}
	suite.Equal(int64(2), total) // the two metro bands; NOT the Chicago tourer
	suite.True(names["Minneapolis Band"])
	suite.True(names["Bloomington Band"], "a metro-resident band with no local show is still rostered")
	suite.False(names["Chicago Tourer"], "a touring act based in another metro is excluded")
}

// TestScene_MetroMemberNullMetroRostered (PSY-1237): a band based in a CBSA member
// city but missing artists.metro still appears on the metro scene roster.
func (suite *SceneServiceIntegrationTestSuite) TestScene_MetroMemberNullMetroRostered() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Brooklyn Bowl", "Brooklyn", "NY")
	v2 := suite.createVerifiedVenue("Elsewhere", "Brooklyn", "NY")

	headliner := suite.createArtistIn("NYC Headliner", "New York City", "NY")
	suite.createArtistInNullMetro("Brooklyn Null Metro", "Brooklyn", "NY")
	suite.createArtistIn("LA Tourer", "Los Angeles", "CA")

	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("BK 1", v1.ID, headliner.ID, user.ID, future)
	suite.createApprovedShow("BK 2", v2.ID, headliner.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("BK 3", v1.ID, headliner.ID, user.ID, future.AddDate(0, 0, 2))

	roster, total, err := suite.sceneService.GetActiveArtists("New York City", "NY", 180, 50, 0)
	suite.Require().NoError(err)
	names := map[string]bool{}
	for _, a := range roster {
		names[a.Name] = true
	}
	suite.Equal(int64(2), total, "NYC metro roster includes NULL-metro Brooklyn member")
	suite.True(names["Brooklyn Null Metro"], "NULL-metro artist in a CBSA member city is rostered")
	suite.True(names["NYC Headliner"])
	suite.False(names["LA Tourer"])
}

// TestScene_MetroMemberAbbrevVariantNullMetro (PSY-1237): contributor "St. Paul" matches
// dataset "Saint Paul" via placeMatchBindVariants in the NULL-metro OR branch.
func (suite *SceneServiceIntegrationTestSuite) TestScene_MetroMemberAbbrevVariantNullMetro() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("First Avenue", "Minneapolis", "MN")
	v2 := suite.createVerifiedVenue("Turf Club", "Saint Paul", "MN")

	headliner := suite.createArtistIn("Mpls Headliner", "Minneapolis", "MN")
	suite.createArtistInNullMetro("St Paul Null Metro", "St. Paul", "MN")

	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("TC 1", v1.ID, headliner.ID, user.ID, future)
	suite.createApprovedShow("TC 2", v2.ID, headliner.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("TC 3", v1.ID, headliner.ID, user.ID, future.AddDate(0, 0, 2))

	_, total, err := suite.sceneService.GetActiveArtists("Minneapolis", "MN", 180, 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
}

// TestScene_NoCBSAFallback verifies a place with no Census CBSA keeps the literal
// (city, state) keying, so non-US / no-CBSA scenes still work (the globe bets on
// global growth — PSY-1255 step C).
func (suite *SceneServiceIntegrationTestSuite) TestScene_NoCBSAFallback() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Club One", "Faketown", "ZZ")
	v2 := suite.createVerifiedVenue("Club Two", "Faketown", "ZZ")
	band := suite.createArtistIn("Faketown Band", "Faketown", "ZZ")
	suite.Require().Nil(v1.Metro, "a no-CBSA place has a NULL metro")

	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("F1", v1.ID, band.ID, user.ID, future)
	suite.createApprovedShow("F2", v2.ID, band.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("F3", v1.ID, band.ID, user.ID, future.AddDate(0, 0, 2))

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().Len(scenes, 1)
	suite.Equal("Faketown", scenes[0].City)
	suite.Equal("ZZ", scenes[0].State)

	detail, err := suite.sceneService.GetSceneDetail("Faketown", "ZZ")
	suite.Require().NoError(err)
	suite.Equal(1, detail.Stats.ArtistCount)

	roster, total, err := suite.sceneService.GetActiveArtists("Faketown", "ZZ", 180, 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(roster, 1)
	suite.Equal("Faketown Band", roster[0].Name)
}

// TestScene_NoCBSAFallback_MixedCase pins the adversarial-review fix: the
// ListScenes fallback grouping and the detail/existence gate must match
// case-insensitively, or a no-CBSA scene whose venues are stored with
// inconsistent casing would LIST (case-insensitive group) but 404 on click
// (case-sensitive gate). Two venues "Faketown"/"faketown" must be one scene that
// resolves on its detail page.
func (suite *SceneServiceIntegrationTestSuite) TestScene_NoCBSAFallback_MixedCase() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Club One", "Faketown", "ZZ")
	v2 := suite.createVerifiedVenue("Club Two", "faketown", "ZZ") // same place, different casing
	band := suite.createArtistIn("Mixed Case Band", "FAKETOWN", "ZZ")

	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("M1", v1.ID, band.ID, user.ID, future)
	suite.createApprovedShow("M2", v2.ID, band.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("M3", v1.ID, band.ID, user.ID, future.AddDate(0, 0, 2))

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)
	suite.Require().Len(scenes, 1)
	suite.Equal(2, scenes[0].VenueCount) // both venues rolled together despite casing

	// The detail gate must AGREE — the listed scene must not 404 on click.
	detail, err := suite.sceneService.GetSceneDetail(scenes[0].City, scenes[0].State)
	suite.Require().NoError(err)
	suite.Equal(2, detail.Stats.VenueCount)
	suite.Equal(1, detail.Stats.ArtistCount) // the mixed-case-home band matches case-insensitively
}

// =============================================================================
// GetSceneDetail Tests
// =============================================================================

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_Success() {
	suite.seedSceneData()

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().NotNil(detail)

	suite.Equal("Phoenix", detail.City)
	suite.Equal("AZ", detail.State)
	suite.Equal("phoenix-az", detail.Slug)
	suite.Nil(detail.Description) // no registry row materialized for this scene

	// Stats
	suite.GreaterOrEqual(detail.Stats.VenueCount, 1)
	suite.GreaterOrEqual(detail.Stats.ArtistCount, 1)
	suite.GreaterOrEqual(detail.Stats.UpcomingShowCount, 1)

	// Pulse
	suite.NotNil(detail.Pulse.ShowsByMonth)
	suite.Len(detail.Pulse.ShowsByMonth, 6)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_NotFound() {
	detail, err := suite.sceneService.GetSceneDetail("Nonexistent", "XX")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "scene not found")
	suite.Nil(detail)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_VenueCountOnlyVerified() {
	suite.seedSceneData()
	// Add an unverified venue — should not be counted
	suite.createUnverifiedVenue("Sketchy Bar", "Phoenix", "AZ")

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Equal(3, detail.Stats.VenueCount) // only the 3 verified ones
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_ArtistCount() {
	_, artists := suite.seedSceneData()
	// seedSceneData creates 3 artists across 5 shows
	_ = artists

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Equal(3, detail.Stats.ArtistCount) // 3 distinct artists
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_FestivalCount() {
	suite.seedSceneData()
	suite.createFestival("M3F Fest", "Phoenix", "AZ")
	suite.createFestival("Arizona Roots", "Phoenix", "AZ")

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Equal(2, detail.Stats.FestivalCount)
}

// TestGetSceneDetail_FestivalCountMetroRollup is the PSY-1278 payoff: a festival
// held in a metro MEMBER city (Tempe → Phoenix CBSA) counts toward the metro's
// scene, while a festival in another metro does not.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_FestivalCountMetroRollup() {
	suite.seedSceneData()
	suite.createFestival("M3F Fest", "Phoenix", "AZ")
	suite.createFestival("Tempe Beach Fest", "Tempe", "AZ")  // Phoenix-CBSA member city
	suite.createFestival("Denver Riot Fest", "Denver", "CO") // different metro entirely

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Equal(2, detail.Stats.FestivalCount, "principal-city + member-city festivals count; other metros don't")
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_PulseShowsByMonth() {
	// Create shows across different months
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("V1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("V2", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("V3", "Phoenix", "AZ")
	a := suite.createArtist("Monthly Band")

	now := time.Now().UTC()
	thisMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// Create shows in current month (count as upcoming too for threshold)
	for i := 0; i < 3; i++ {
		// Use dates in the future portion of this month
		showDate := thisMonthStart.AddDate(0, 1, -1) // last day of this month
		suite.createApprovedShow(
			fmt.Sprintf("This Month Show %d", i),
			[]*catalogm.Venue{v1, v2, v3}[i%3].ID, a.ID, user.ID,
			showDate,
		)
	}

	// Create shows in previous month
	prevMonth := thisMonthStart.AddDate(0, -1, 5)
	suite.createApprovedShow("Prev Month Show 1", v1.ID, a.ID, user.ID, prevMonth)
	suite.createApprovedShow("Prev Month Show 2", v2.ID, a.ID, user.ID, prevMonth.AddDate(0, 0, 1))

	// Also create upcoming shows to meet threshold
	future := now.AddDate(0, 0, 7)
	suite.createApprovedShow("Future 1", v1.ID, a.ID, user.ID, future)
	suite.createApprovedShow("Future 2", v2.ID, a.ID, user.ID, future.AddDate(0, 0, 1))

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)

	// Shows by month should have 6 entries
	suite.Len(detail.Pulse.ShowsByMonth, 6)
	// Last entry (index 5) is current month — should have 3+ shows
	suite.GreaterOrEqual(detail.Pulse.ShowsByMonth[5], 3)
	// Second to last (index 4) is previous month — should have 2 shows
	suite.Equal(2, detail.Pulse.ShowsByMonth[4])
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_PulseShowsTrend() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Venue 1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("Venue 2", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("Venue 3", "Phoenix", "AZ")
	a := suite.createArtist("Trend Band")

	now := time.Now().UTC()
	thisMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// 5 shows this month
	for i := 0; i < 5; i++ {
		showDate := thisMonthStart.AddDate(0, 1, -1)
		suite.createApprovedShow(
			fmt.Sprintf("This Month %d", i),
			[]*catalogm.Venue{v1, v2, v3}[i%3].ID, a.ID, user.ID,
			showDate,
		)
	}

	// 2 shows previous month
	prevMonth := thisMonthStart.AddDate(0, -1, 5)
	suite.createApprovedShow("Prev 1", v1.ID, a.ID, user.ID, prevMonth)
	suite.createApprovedShow("Prev 2", v2.ID, a.ID, user.ID, prevMonth.AddDate(0, 0, 1))

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)

	suite.Equal("+3", detail.Pulse.ShowsTrend) // 5 - 2 = +3
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_PulseNewArtists() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("PNV1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("PNV2", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("PNV3", "Phoenix", "AZ")

	// Old artist — first show 60 days ago
	oldArtist := suite.createArtist("Old Band")
	past := time.Now().UTC().AddDate(0, 0, -60)
	suite.createApprovedShow("Old Show", v1.ID, oldArtist.ID, user.ID, past)

	// New artist — first show 10 days ago
	newArtist := suite.createArtist("New Band")
	recent := time.Now().UTC().AddDate(0, 0, -10)
	suite.createApprovedShow("New Show", v2.ID, newArtist.ID, user.ID, recent)

	// Another new artist — first show 5 days ago
	newerArtist := suite.createArtist("Newer Band")
	moreRecent := time.Now().UTC().AddDate(0, 0, -5)
	suite.createApprovedShow("Newer Show", v3.ID, newerArtist.ID, user.ID, moreRecent)

	// Need 5+ upcoming shows for threshold
	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("F1", v1.ID, oldArtist.ID, user.ID, future)
	suite.createApprovedShow("F2", v2.ID, newArtist.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("F3", v3.ID, newerArtist.ID, user.ID, future.AddDate(0, 0, 2))
	suite.createApprovedShow("F4", v1.ID, newArtist.ID, user.ID, future.AddDate(0, 0, 3))
	suite.createApprovedShow("F5", v2.ID, oldArtist.ID, user.ID, future.AddDate(0, 0, 4))

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)

	// 2 new artists (first show in last 30 days)
	suite.Equal(2, detail.Pulse.NewArtists30d)
}

// =============================================================================
// GetSceneDetail rooms-leaderboard Tests (PSY-1780)
// =============================================================================

// The leaderboard's headline claim: busiest room first, by UPCOMING shows.
// seedSceneData books v1 twice, v2 twice and v3 once, so a third booking at v1
// breaks the v1/v2 tie and leaves one order that can pass.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_VenuesRankedByUpcomingCount() {
	venues, artists := suite.seedSceneData()
	user := suite.createUser()
	suite.createApprovedShow("Extra At Crescent", venues[0].ID, artists[0].ID, user.ID,
		time.Now().UTC().AddDate(0, 0, 9))

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().Len(detail.Venues, 3)

	suite.Equal("Crescent Ballroom", detail.Venues[0].Name)
	suite.Equal(3, detail.Venues[0].UpcomingShowCount)
	suite.Equal("Valley Bar", detail.Venues[1].Name)
	suite.Equal(2, detail.Venues[1].UpcomingShowCount)
	suite.Equal("The Rebel Lounge", detail.Venues[2].Name)
	suite.Equal(1, detail.Venues[2].UpcomingShowCount)

	// The room's own city rides along — the leaderboard has to say where a
	// reader would be going, and it is not always the scene's principal city.
	suite.Equal("Phoenix", detail.Venues[0].City)
}

// A tracked room with nothing booked is still a room the scene covers. It ranks
// last, but dropping it would quietly redefine the list as "rooms with shows" —
// and it is the whole shape of a sparse scene.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_VenuesIncludeZeroCountRooms() {
	suite.seedSceneData()
	suite.createVerifiedVenue("Quiet Room", "Phoenix", "AZ")

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().Len(detail.Venues, 4)

	last := detail.Venues[3]
	suite.Equal("Quiet Room", last.Name)
	suite.Equal(0, last.UpcomingShowCount)
}

// The sparse case end to end: a scene that clears the venue gate but has nothing
// upcoming anywhere. The leaderboard must still come back — every room, every
// count zero, alphabetical — rather than empty or nil.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_VenuesOnSparseScene() {
	// One room clear of the existence gate, so raising sceneMinVenues later
	// fails the tests that are ABOUT the gate rather than this one, which would
	// otherwise die as a bare ErrSceneNotFound pointing nowhere near a
	// leaderboard.
	suite.Require().Equal(2, sceneMinVenues, "fixture sized against the gate")
	suite.createVerifiedVenue("Zebra Room", "Phoenix", "AZ")
	suite.createVerifiedVenue("Aardvark Hall", "Phoenix", "AZ")
	suite.createVerifiedVenue("Mango Lounge", "Phoenix", "AZ")

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().NotNil(detail.Venues)
	suite.Require().Len(detail.Venues, 3)

	// All tied at zero, so this also pins the name tiebreak.
	suite.Equal("Aardvark Hall", detail.Venues[0].Name)
	suite.Equal(0, detail.Venues[0].UpcomingShowCount)
	suite.Equal("Mango Lounge", detail.Venues[1].Name)
	suite.Equal("Zebra Room", detail.Venues[2].Name)
	suite.Equal(0, detail.Venues[2].UpcomingShowCount)
}

// The list is capped to the TRACKED set, which is the same set the day payload
// names: verified rooms only. An unverified room with a packed calendar would
// otherwise top the leaderboard on a page that claims to cover curated rooms.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_VenuesExcludeUnverifiedRooms() {
	_, artists := suite.seedSceneData()
	user := suite.createUser()
	sketchy := suite.createUnverifiedVenue("Sketchy Bar", "Phoenix", "AZ")
	future := time.Now().UTC().AddDate(0, 0, 5)
	suite.createApprovedShow("Sketchy 1", sketchy.ID, artists[0].ID, user.ID, future)
	suite.createApprovedShow("Sketchy 2", sketchy.ID, artists[1].ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("Sketchy 3", sketchy.ID, artists[2].ID, user.ID, future.AddDate(0, 0, 2))
	suite.createApprovedShow("Sketchy 4", sketchy.ID, artists[0].ID, user.ID, future.AddDate(0, 0, 3))

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().Len(detail.Venues, 3)
	for _, v := range detail.Venues {
		suite.NotEqual("Sketchy Bar", v.Name)
	}
	// The leaderboard and the headline venue_count now count through ONE
	// predicate, so this equality is structural rather than a coincidence.
	suite.Equal(detail.Stats.VenueCount, len(detail.Venues))

	// And it pins the divergence the contract spends a paragraph defending: the
	// scene TOTAL still counts the unverified room's 4 shows (5 seeded + 4), so
	// the per-room counts fall short of it on purpose. Without this, someone
	// "reconciles" the two numbers and silently changes a published figure.
	suite.Equal(9, detail.Stats.UpcomingShowCount)
	roomSum := 0
	for _, v := range detail.Venues {
		roomSum += v.UpcomingShowCount
	}
	suite.Equal(5, roomSum)
}

// The id tiebreak, which nothing else exercises: two rooms of the same name in
// one metro are legal (uniqueness is per city), and with both on zero the only
// thing separating them is `v.id ASC`. Drop it and the order goes
// implementation-defined with the rest of the suite still green.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_VenuesOrderIsStableForDuplicateNames() {
	first := suite.createVerifiedVenue("The Lounge", "Phoenix", "AZ")
	second := suite.createVerifiedVenue("The Lounge", "Tempe", "AZ") // same name, different city
	suite.Require().NotEqual(first.ID, second.ID)

	for i := 0; i < 2; i++ {
		detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
		suite.Require().NoError(err)
		suite.Require().Len(detail.Venues, 2)
		suite.Equal("The Lounge", detail.Venues[0].Name)
		suite.Equal("The Lounge", detail.Venues[1].Name)
		// Identical names and identical zero counts — id is the only thing left.
		suite.Equal(first.ID, detail.Venues[0].ID, "call %d", i)
		suite.Equal(second.ID, detail.Venues[1].ID, "call %d", i)
		// The rows are distinguishable on the wire, which is the whole point of
		// shipping an id: without it these two are byte-identical objects.
		suite.Equal("Phoenix", detail.Venues[0].City)
		suite.Equal("Tempe", detail.Venues[1].City)
	}
}

// A show billed to TWO tracked rooms counts for both, so the per-room counts sum
// past the scene total. The contract says so; this is what stops a future reader
// from "fixing" the double count.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_VenueCountsDoubleCountSharedBills() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Room One", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("Room Two", "Phoenix", "AZ")
	band := suite.createArtist("Split Bill Band")

	show := suite.createApprovedShow("Two Room Fest", v1.ID, band.ID, user.ID,
		time.Now().UTC().AddDate(0, 0, 3))
	// The same show, billed to the second room as well.
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: v2.ID}).Error)

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().Len(detail.Venues, 2)

	suite.Equal(1, detail.Venues[0].UpcomingShowCount)
	suite.Equal(1, detail.Venues[1].UpcomingShowCount)
	// One show, counted once by the scene and twice across the rooms.
	suite.Equal(1, detail.Stats.UpcomingShowCount)
}

// A date-only show is stored at UTC midnight, so an instant bound would drop
// TODAY's shows the moment that midnight passes — a room whose only booking is
// tonight would read 0 and rank last. This is the shape of a production row, not
// the mid-afternoon instants the other fixtures use.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_VenueCountsIncludeTodaysMidnightShows() {
	user := suite.createUser()
	v := suite.createVerifiedVenue("Tonight Only", "Phoenix", "AZ")
	suite.createVerifiedVenue("Dark Tonight", "Phoenix", "AZ")
	band := suite.createArtist("Tonight Band")

	// Midnight UTC today: already in the past as an instant, still to come as a date.
	todayMidnight := time.Now().UTC().Truncate(24 * time.Hour)
	suite.Require().True(todayMidnight.Before(time.Now().UTC()), "the bug only bites once the instant has passed")
	suite.createApprovedShow("Tonight Show", v.ID, band.ID, user.ID, todayMidnight)

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().Len(detail.Venues, 2)

	suite.Equal("Tonight Only", detail.Venues[0].Name)
	suite.Equal(1, detail.Venues[0].UpcomingShowCount, "tonight's show must still count")
	suite.Equal("Dark Tonight", detail.Venues[1].Name)
	suite.Equal(0, detail.Venues[1].UpcomingShowCount)
}

// UPCOMING, not lifetime. A room whose entire history is behind it reads zero,
// so the leaderboard answers "where is something on" rather than "which room has
// been around longest" — the roster already ranks by total shows.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_VenueCountsExcludePastShows() {
	venues, artists := suite.seedSceneData()
	user := suite.createUser()
	faded := suite.createVerifiedVenue("Faded Palace", "Phoenix", "AZ")
	past := time.Now().UTC().AddDate(0, 0, -30)
	suite.createApprovedShow("Long Gone 1", faded.ID, artists[0].ID, user.ID, past)
	suite.createApprovedShow("Long Gone 2", faded.ID, artists[1].ID, user.ID, past.AddDate(0, 0, 1))
	// A past show at a room that also has upcoming ones must not inflate it.
	suite.createApprovedShow("Old Crescent", venues[0].ID, artists[0].ID, user.ID, past)

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)

	byName := map[string]int{}
	for _, v := range detail.Venues {
		byName[v.Name] = v.UpcomingShowCount
	}
	suite.Equal(0, byName["Faded Palace"])
	suite.Equal(2, byName["Crescent Ballroom"])
}

// A metro scene's rooms are the METRO's rooms: a Tempe venue belongs to the
// Phoenix scene, and its City field says Tempe, not Phoenix.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_VenuesRollUpMemberCities() {
	_, artists := suite.seedSceneData()
	user := suite.createUser()
	tempe := suite.createVerifiedVenue("Yucca Tap Room", "Tempe", "AZ") // Phoenix-CBSA member city
	// Assert the premise, so a geo-dataset change fails as "Tempe stopped
	// resolving" rather than as an unexplained off-by-one in the row count.
	suite.Require().NotNil(tempe.Metro, "Tempe must resolve to the Phoenix CBSA")
	future := time.Now().UTC().AddDate(0, 0, 2)
	suite.createApprovedShow("Tempe 1", tempe.ID, artists[0].ID, user.ID, future)
	suite.createApprovedShow("Tempe 2", tempe.ID, artists[1].ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("Tempe 3", tempe.ID, artists[2].ID, user.ID, future.AddDate(0, 0, 2))

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().Len(detail.Venues, 4)

	suite.Equal("Yucca Tap Room", detail.Venues[0].Name)
	suite.Equal(3, detail.Venues[0].UpcomingShowCount)
	suite.Equal("Tempe", detail.Venues[0].City)
}

// Each room has to be linkable. Website is genuinely absent for most production
// rows; slug is backfilled for all of them but still NULLABLE in the schema, and
// this fixture bypasses the service that generates one. So both columns exercise
// the projection that turns a NULL into "", alongside the pass-through of a set
// value: a client distinguishes "render it unlinked" from "link it" on the empty
// string alone.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_VenuesCarryLinkFields() {
	venues, _ := suite.seedSceneData()
	suite.Require().NoError(suite.db.Model(venues[0]).Updates(map[string]any{
		"slug":    "crescent-ballroom",
		"website": "https://crescentphx.com",
	}).Error)

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().Len(detail.Venues, 3)

	byName := map[string]contracts.SceneVenueSummary{}
	for _, v := range detail.Venues {
		byName[v.Name] = v
	}
	suite.Equal("crescent-ballroom", byName["Crescent Ballroom"].Slug)
	suite.Equal("https://crescentphx.com", byName["Crescent Ballroom"].Website)
	// The untouched rooms keep NULL in both columns and must come back as "",
	// not as a scan error and not as "null".
	suite.Equal("", byName["Valley Bar"].Slug)
	suite.Equal("", byName["Valley Bar"].Website)
}

// The no-CBSA branch, which every other leaderboard test misses: Phoenix
// resolves to a metro, so those all exercise the one-arg venue predicate. The
// fallback branch binds TWO args, and the leaderboard's own placeholders are
// bound BEFORE them (the counts are aggregated in a derived table that precedes
// the WHERE). A positional-binding slip would land here and nowhere else —
// silently counting a scene's rooms against the wrong city, or erroring outright.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_VenuesOnNoCBSAFallbackScene() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Club One", "Faketown", "ZZ")
	v2 := suite.createVerifiedVenue("Club Two", "Faketown", "ZZ")
	suite.Require().Nil(v1.Metro, "a no-CBSA place has a NULL metro")
	band := suite.createArtistIn("Faketown Band", "Faketown", "ZZ")

	// A same-named room in ANOTHER city must not leak in: the fallback predicate
	// keys on (city, state), and venue names are unique only WITHIN a city.
	elsewhere := suite.createVerifiedVenue("Club One", "Othertown", "ZZ")

	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("F1", v1.ID, band.ID, user.ID, future)
	suite.createApprovedShow("F2", v1.ID, band.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("F3", v2.ID, band.ID, user.ID, future.AddDate(0, 0, 2))
	suite.createApprovedShow("F4", elsewhere.ID, band.ID, user.ID, future.AddDate(0, 0, 3))

	detail, err := suite.sceneService.GetSceneDetail("Faketown", "ZZ")
	suite.Require().NoError(err)
	suite.Require().Len(detail.Venues, 2)

	suite.Equal("Club One", detail.Venues[0].Name)
	suite.Equal(2, detail.Venues[0].UpcomingShowCount) // NOT 3 — Othertown's room is a different scene
	suite.Equal("Faketown", detail.Venues[0].City)
	suite.Equal("Club Two", detail.Venues[1].Name)
	suite.Equal(1, detail.Venues[1].UpcomingShowCount)
}

// The leaderboard, the day footer, and the week footer must never name
// different rooms for one city — they read the same definition, and this is the
// assertion that keeps it that way if one query is edited alone.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_VenuesMatchTrackedVenueSet() {
	suite.seedSceneData()
	suite.createVerifiedVenue("Quiet Room", "Phoenix", "AZ")
	suite.createUnverifiedVenue("Sketchy Bar", "Phoenix", "AZ")

	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)

	day, err := suite.sceneService.GetSceneDay("Phoenix", "AZ", "")
	suite.Require().NoError(err)

	week, err := suite.sceneService.GetSceneWeek("Phoenix", "AZ", "")
	suite.Require().NoError(err)

	leaderboard := make([]string, 0, len(detail.Venues))
	for _, v := range detail.Venues {
		leaderboard = append(leaderboard, v.Name)
	}
	dayTracked := make([]string, 0, len(day.TrackedVenues))
	for _, v := range day.TrackedVenues {
		dayTracked = append(dayTracked, v.Name)
	}
	weekTracked := make([]string, 0, len(week.TrackedVenues))
	for _, v := range week.TrackedVenues {
		weekTracked = append(weekTracked, v.Name)
	}
	// Pin the size first: ElementsMatch of two EMPTY lists passes, so a shared
	// predicate broken to match nothing would turn the one test whose whole job
	// is "these two surfaces agree" into a green vacuous truth.
	suite.Require().Len(leaderboard, 4)
	suite.ElementsMatch(dayTracked, leaderboard)
	suite.ElementsMatch(weekTracked, leaderboard)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneWeek_TrackedVenuesCarrySlug() {
	venues, _ := suite.seedSceneData()
	const slug = "crescent-ballroom"
	suite.Require().NoError(suite.db.Model(venues[0]).Update("slug", slug).Error)

	week, err := suite.sceneService.GetSceneWeek("Phoenix", "AZ", "")
	suite.Require().NoError(err)

	var found *contracts.SceneTrackedVenue
	for i := range week.TrackedVenues {
		if week.TrackedVenues[i].Name == venues[0].Name {
			found = &week.TrackedVenues[i]
			break
		}
	}
	suite.Require().NotNil(found, "week payload must include the slugged room")
	suite.Equal(slug, found.Slug)
}

// =============================================================================
// GetActiveArtists Tests
// =============================================================================

func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_Success() {
	_, artists := suite.seedSceneData()
	// Band A has 2 shows (at v1 and v2), Band B has 2 shows (at v1 and v3), Band C has 1 show (at v2)
	_ = artists

	results, total, err := suite.sceneService.GetActiveArtists("Phoenix", "AZ", 365, 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Len(results, 3)

	// First should be highest show count (Band A or Band B, both have 2)
	suite.Equal(2, results[0].ShowCount)
	suite.Equal(2, results[1].ShowCount)
	suite.Equal(1, results[2].ShowCount)
}

// PSY-1224: the roster carries each artist's bandcamp_embed_url so the /atlas
// preview can play one as the scene's instant-payoff track. A set URL passes
// through verbatim; an absent one stays nil (no synthesized URL).
func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_IncludesBandcampEmbedURL() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")

	const embedURL = "https://hasembed.bandcamp.com/album/debut"
	withEmbed := &catalogm.Artist{
		Name:             "Has Embed",
		City:             stringPtr("Phoenix"),
		State:            stringPtr("AZ"),
		Metro:            seedMetro("Phoenix", "AZ"),
		BandcampEmbedURL: stringPtr(embedURL),
	}
	suite.Require().NoError(suite.db.Create(withEmbed).Error)
	withoutEmbed := suite.createArtistIn("No Embed", "Phoenix", "AZ")

	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("E1", v1.ID, withEmbed.ID, user.ID, future)
	suite.createApprovedShow("E2", v2.ID, withoutEmbed.ID, user.ID, future)

	results, _, err := suite.sceneService.GetActiveArtists("Phoenix", "AZ", 365, 20, 0)
	suite.Require().NoError(err)

	var hasEmbedFound, noEmbedFound bool
	var hasEmbedURL, noEmbedURL *string
	for _, r := range results {
		switch r.Name {
		case "Has Embed":
			hasEmbedFound, hasEmbedURL = true, r.BandcampEmbedURL
		case "No Embed":
			noEmbedFound, noEmbedURL = true, r.BandcampEmbedURL
		}
	}
	suite.Require().True(hasEmbedFound, "the artist with an embed is in the roster")
	suite.Require().True(noEmbedFound, "the artist without an embed is in the roster")
	suite.Require().NotNil(hasEmbedURL)
	suite.Equal(embedURL, *hasEmbedURL)
	suite.Nil(noEmbedURL, "an artist with no embed passes through as nil")
}

// PSY-1233: a scene's artists are its LOCAL artists (home city/state matches the
// scene), not every touring act that played a venue there. Pins the filter across
// GetActiveArtists (list + total) and the scene-detail artist count.
func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_ExcludesTouringActs() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Crescent Ballroom", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")

	local := suite.createArtistIn("Phoenix Local", "Phoenix", "AZ")
	touring := suite.createArtistIn("LA Tourer", "Los Angeles", "CA")
	// Local despite contributor free-text casing/whitespace (case-insensitive + trimmed match).
	messy := suite.createArtistIn("Messy Casing", "  phoenix ", " az ")
	// NULL home city → can't be claimed as local → excluded.
	noCity := &catalogm.Artist{Name: "No Home City"}
	suite.Require().NoError(suite.db.Create(noCity).Error)

	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("Local 1", v1.ID, local.ID, user.ID, future)
	suite.createApprovedShow("Local 2", v2.ID, local.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("Touring", v1.ID, touring.ID, user.ID, future.AddDate(0, 0, 2))
	suite.createApprovedShow("Messy", v2.ID, messy.ID, user.ID, future.AddDate(0, 0, 3))
	suite.createApprovedShow("NoCity", v1.ID, noCity.ID, user.ID, future.AddDate(0, 0, 4))

	results, total, err := suite.sceneService.GetActiveArtists("Phoenix", "AZ", 365, 20, 0)
	suite.Require().NoError(err)

	names := make([]string, 0, len(results))
	for _, r := range results {
		names = append(names, r.Name)
	}
	suite.Equal(int64(2), total, "only the two LOCAL artists count toward the scene")
	suite.ElementsMatch([]string{"Phoenix Local", "Messy Casing"}, names)
	suite.NotContains(names, "LA Tourer", "a touring act based elsewhere is excluded")
	suite.NotContains(names, "No Home City", "an artist with no home city can't be claimed as local")

	// The scene-detail artist count uses the same filter.
	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Equal(2, detail.Stats.ArtistCount)
	// ...and so does the new-artists-30d pulse: all five acts have a recent first
	// show, but only the two locals count.
	suite.Equal(2, detail.Pulse.NewArtists30d)
}

// TestGetSceneGenreDistribution_ExcludesTouringActs (PSY-1233): the scene's genre
// distribution reflects LOCAL artists. A touring act's genre tag must not pollute
// the scene even though it played a venue in the city.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGenreDistribution_ExcludesTouringActs() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("GX-V1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("GX-V2", "Phoenix", "AZ")
	venues := []*catalogm.Venue{v1, v2}

	punkTag := suite.createGenreTag("punk", "punk")
	jazzTag := suite.createGenreTag("jazz", "jazz")
	future := time.Now().UTC().AddDate(0, 0, 7)

	// 30 LOCAL punk artists — meets the 30-tagged-artist threshold.
	for i := 0; i < 30; i++ {
		a := suite.createArtist(fmt.Sprintf("Local Punk %d", i)) // Phoenix-local (default)
		suite.createApprovedShow(fmt.Sprintf("LP Show %d", i), venues[i%2].ID, a.ID, user.ID, future.AddDate(0, 0, i))
		suite.tagArtist(a.ID, punkTag, user.ID)
	}
	// A touring jazz act playing a Phoenix venue — its genre must NOT appear.
	tourer := suite.createArtistIn("LA Jazz Tourer", "Los Angeles", "CA")
	suite.createApprovedShow("Tour Show", v1.ID, tourer.ID, user.ID, future)
	suite.tagArtist(tourer.ID, jazzTag, user.ID)

	genres, err := suite.sceneService.GetSceneGenreDistribution("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().NotEmpty(genres)
	names := make([]string, 0, len(genres))
	for _, g := range genres {
		names = append(names, g.Name)
	}
	suite.Contains(names, "punk", "local artists' genre is present")
	suite.NotContains(names, "jazz", "a touring act's genre must not pollute the scene")
}

func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_RespectsLimit() {
	suite.seedSceneData()

	results, total, err := suite.sceneService.GetActiveArtists("Phoenix", "AZ", 365, 2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Len(results, 2)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_RespectsOffset() {
	suite.seedSceneData()

	results, total, err := suite.sceneService.GetActiveArtists("Phoenix", "AZ", 365, 20, 2)
	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Len(results, 1) // 3 total, offset 2 = 1 remaining
}

func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_Period() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("Period V1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("Period V2", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("Period V3", "Phoenix", "AZ")

	recentArtist := suite.createArtist("Recent Artist")
	oldArtist := suite.createArtist("Old Artist")

	// Recent show (10 days ago)
	recent := time.Now().UTC().AddDate(0, 0, -10)
	suite.createApprovedShow("Recent Show", v1.ID, recentArtist.ID, user.ID, recent)

	// Old show (100 days ago — outside 90 day period)
	old := time.Now().UTC().AddDate(0, 0, -100)
	suite.createApprovedShow("Old Show", v2.ID, oldArtist.ID, user.ID, old)

	// Need upcoming shows for the scene threshold
	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("F1", v1.ID, recentArtist.ID, user.ID, future)
	suite.createApprovedShow("F2", v2.ID, recentArtist.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("F3", v3.ID, recentArtist.ID, user.ID, future.AddDate(0, 0, 2))
	suite.createApprovedShow("F4", v1.ID, recentArtist.ID, user.ID, future.AddDate(0, 0, 3))
	suite.createApprovedShow("F5", v2.ID, recentArtist.ID, user.ID, future.AddDate(0, 0, 4))

	// Period is the ACTIVE WINDOW now, not a membership gate (PSY-1255 step C):
	// the roster is every band BASED in the Phoenix metro, with the ones active in
	// the window (or upcoming) flagged and sorted first. recentArtist has a show
	// within 90 days (and upcoming) → active; oldArtist's only show was 100 days
	// ago → inactive, but still part of the roster.
	results, total, err := suite.sceneService.GetActiveArtists("Phoenix", "AZ", 90, 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Require().Len(results, 2)
	suite.Equal("Recent Artist", results[0].Name)
	suite.True(results[0].IsActive, "recentArtist should be active")
	suite.Equal("Old Artist", results[1].Name)
	suite.False(results[1].IsActive, "oldArtist should be inactive but rostered")
}

func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_NotFound() {
	results, total, err := suite.sceneService.GetActiveArtists("Nowhere", "XX", 90, 20, 0)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "scene not found")
	suite.Nil(results)
	suite.Zero(total)
}

// =============================================================================
// GetRepresentativeEmbed Tests (PSY-1294)
// =============================================================================

// The representative embed is chosen over the FULL roster: it surfaces an
// embed-having band even when a HIGHER-ranked band (more shows) has no embed —
// the coverage gap the client-side, window-capped pick left open.
func (suite *SceneServiceIntegrationTestSuite) TestGetRepresentativeEmbed_PicksEmbedPastHigherRankedBand() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("RE V1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("RE V2", "Phoenix", "AZ")

	// Top-ranked active band (most shows) has NO embed — it must be skipped.
	topNoEmbed := suite.createArtistIn("AAA Top No Embed", "Phoenix", "AZ")
	// Lower-ranked active band (fewer shows) HAS an embed.
	const embedURL = "https://withembed.bandcamp.com/album/x"
	withEmbed := &catalogm.Artist{
		Name:             "ZZZ With Embed",
		Slug:             stringPtr("zzz-with-embed"),
		City:             stringPtr("Phoenix"),
		State:            stringPtr("AZ"),
		Metro:            seedMetro("Phoenix", "AZ"),
		BandcampEmbedURL: stringPtr(embedURL),
	}
	suite.Require().NoError(suite.db.Create(withEmbed).Error)

	future := time.Now().UTC().AddDate(0, 0, 7)
	// topNoEmbed gets more shows → outranks withEmbed in the active-first order.
	suite.createApprovedShow("T1", v1.ID, topNoEmbed.ID, user.ID, future)
	suite.createApprovedShow("T2", v2.ID, topNoEmbed.ID, user.ID, future.AddDate(0, 0, 1))
	suite.createApprovedShow("W1", v1.ID, withEmbed.ID, user.ID, future.AddDate(0, 0, 2))

	embed, err := suite.sceneService.GetRepresentativeEmbed("Phoenix", "AZ", 180)
	suite.Require().NoError(err)
	suite.Require().NotNil(embed, "an embed-having band exists in the roster")
	suite.Equal(embedURL, embed.EmbedURL)
	suite.Equal("ZZZ With Embed", embed.ArtistName)
	suite.Equal("zzz-with-embed", embed.ArtistSlug)
}

// Active-first: with two embed-having bands, the ACTIVE one wins even when the
// inactive one sorts first alphabetically.
func (suite *SceneServiceIntegrationTestSuite) TestGetRepresentativeEmbed_PrefersActiveBand() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("RE2 V1", "Phoenix", "AZ")
	// Second verified venue only exists to clear the scene threshold (2 venues).
	suite.createVerifiedVenue("RE2 V2", "Phoenix", "AZ")

	// Inactive band (no show) whose name sorts BEFORE the active one — proving
	// is_active DESC beats the name tiebreak.
	inactiveWithEmbed := &catalogm.Artist{
		Name:             "AAA Inactive Embed",
		Slug:             stringPtr("aaa-inactive-embed"),
		City:             stringPtr("Phoenix"),
		State:            stringPtr("AZ"),
		Metro:            seedMetro("Phoenix", "AZ"),
		BandcampEmbedURL: stringPtr("https://inactive.bandcamp.com/album/x"),
	}
	suite.Require().NoError(suite.db.Create(inactiveWithEmbed).Error)

	const activeURL = "https://active.bandcamp.com/album/y"
	activeWithEmbed := &catalogm.Artist{
		Name:             "ZZZ Active Embed",
		Slug:             stringPtr("zzz-active-embed"),
		City:             stringPtr("Phoenix"),
		State:            stringPtr("AZ"),
		Metro:            seedMetro("Phoenix", "AZ"),
		BandcampEmbedURL: stringPtr(activeURL),
	}
	suite.Require().NoError(suite.db.Create(activeWithEmbed).Error)

	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("A1", v1.ID, activeWithEmbed.ID, user.ID, future)

	embed, err := suite.sceneService.GetRepresentativeEmbed("Phoenix", "AZ", 180)
	suite.Require().NoError(err)
	suite.Require().NotNil(embed)
	suite.Equal(activeURL, embed.EmbedURL, "the ACTIVE band wins over the inactive one")
	suite.Equal("ZZZ Active Embed", embed.ArtistName)
}

// PSY-1294 decision "active-first, else any": when NO active band has an embed
// but a dormant one does, the dormant band is the fallback (not silence). Also
// pins scope — a touring act based elsewhere with an embed is NOT chosen.
func (suite *SceneServiceIntegrationTestSuite) TestGetRepresentativeEmbed_FallsBackToDormantAndRespectsScope() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("RE3 V1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("RE3 V2", "Phoenix", "AZ")

	// The only ACTIVE Phoenix band has no embed.
	activeNoEmbed := suite.createArtistIn("Active No Embed", "Phoenix", "AZ")
	future := time.Now().UTC().AddDate(0, 0, 7)
	suite.createApprovedShow("AN1", v1.ID, activeNoEmbed.ID, user.ID, future)

	// A dormant (no show) Phoenix band DOES have an embed — the fallback.
	const dormantURL = "https://dormant.bandcamp.com/album/x"
	dormantLocal := &catalogm.Artist{
		Name:             "Dormant Local",
		Slug:             stringPtr("dormant-local"),
		City:             stringPtr("Phoenix"),
		State:            stringPtr("AZ"),
		Metro:            seedMetro("Phoenix", "AZ"),
		BandcampEmbedURL: stringPtr(dormantURL),
	}
	suite.Require().NoError(suite.db.Create(dormantLocal).Error)

	// A touring act based in LA with an embed must be excluded by scope, even
	// though it played a Phoenix venue (upcoming show) — proving the roster is
	// metro-residence, not played-here.
	tourer := &catalogm.Artist{
		Name:             "LA Tourer Embed",
		Slug:             stringPtr("la-tourer-embed"),
		City:             stringPtr("Los Angeles"),
		State:            stringPtr("CA"),
		Metro:            seedMetro("Los Angeles", "CA"),
		BandcampEmbedURL: stringPtr("https://tourer.bandcamp.com/album/z"),
	}
	suite.Require().NoError(suite.db.Create(tourer).Error)
	suite.createApprovedShow("Tour", v2.ID, tourer.ID, user.ID, future.AddDate(0, 0, 1))

	embed, err := suite.sceneService.GetRepresentativeEmbed("Phoenix", "AZ", 180)
	suite.Require().NoError(err)
	suite.Require().NotNil(embed, "a dormant local band's embed is the fallback")
	suite.Equal(dormantURL, embed.EmbedURL)
	suite.Equal("Dormant Local", embed.ArtistName)
	suite.Equal("dormant-local", embed.ArtistSlug)
}

// A valid scene where no band based here has an embed → nil (the preview shows
// no player), NOT an error.
func (suite *SceneServiceIntegrationTestSuite) TestGetRepresentativeEmbed_NilWhenNoBandHasEmbed() {
	suite.seedSceneData() // 3 venues, 3 bands, none with an embed

	embed, err := suite.sceneService.GetRepresentativeEmbed("Phoenix", "AZ", 180)
	suite.Require().NoError(err)
	suite.Nil(embed, "no embed-having band → nil, not an error")
}

// An unknown scene returns the scene-not-found error, same as the roster query.
func (suite *SceneServiceIntegrationTestSuite) TestGetRepresentativeEmbed_NotFound() {
	embed, err := suite.sceneService.GetRepresentativeEmbed("Nowhere", "XX", 180)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "scene not found")
	suite.Nil(embed)
}

// =============================================================================
// ParseSceneSlug Tests
// =============================================================================

func (suite *SceneServiceIntegrationTestSuite) TestParseSceneSlug_Success() {
	suite.createVerifiedVenue("Test Venue", "Phoenix", "AZ")

	city, state, err := suite.sceneService.ParseSceneSlug("phoenix-az")
	suite.Require().NoError(err)
	suite.Equal("Phoenix", city)
	suite.Equal("AZ", state)
}

func (suite *SceneServiceIntegrationTestSuite) TestParseSceneSlug_MultiWordCity() {
	suite.createVerifiedVenue("Test Venue", "New York", "NY")

	// A multi-word slug resolves to its CBSA metro's PRINCIPAL city (PSY-1255
	// step C): "new-york-ny" pins the NYC metro, whose principal city is the
	// canonical GeoNames "New York City" — so a venue seeded as "New York" still
	// resolves, and the scene displays under the canonical metro identity.
	city, state, err := suite.sceneService.ParseSceneSlug("new-york-ny")
	suite.Require().NoError(err)
	suite.Equal("New York City", city)
	suite.Equal("NY", state)
}

func (suite *SceneServiceIntegrationTestSuite) TestParseSceneSlug_MemberSlugResolvesToPrincipal() {
	// A suburb slug resolves to its metro's PRINCIPAL city (Tempe → Phoenix), so
	// old member URLs land on the canonical metro scene instead of 404ing
	// (PSY-1255 step C). Resolution is purely geographic — no venue seeding needed.
	city, state, err := suite.sceneService.ParseSceneSlug("tempe-az")
	suite.Require().NoError(err)
	suite.Equal("Phoenix", city)
	suite.Equal("AZ", state)
}

func (suite *SceneServiceIntegrationTestSuite) TestParseSceneSlug_NotFound() {
	city, state, err := suite.sceneService.ParseSceneSlug("nonexistent-xx")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "scene not found")
	suite.Empty(city)
	suite.Empty(state)
}

func (suite *SceneServiceIntegrationTestSuite) TestParseSceneSlug_IgnoresUnverifiedVenues() {
	suite.createUnverifiedVenue("Unverified Place", "Unverified City", "UC")

	city, state, err := suite.sceneService.ParseSceneSlug("unverified-city-uc")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "scene not found")
	suite.Empty(city)
	suite.Empty(state)
}

func (suite *SceneServiceIntegrationTestSuite) TestParseSceneSlug_HyphenatedCity() {
	// The SQL fallback must match the stored hyphen, not a space-collapsed
	// re-parse of the slug ("foo bar" would miss "Foo-Bar").
	suite.createVerifiedVenue("Hyphen Room", "Foo-Bar", "ZZ")

	city, state, err := suite.sceneService.ParseSceneSlug("foo-bar-zz")
	suite.Require().NoError(err)
	suite.Equal("Foo-Bar", city)
	suite.Equal("ZZ", state)
}

func (suite *SceneServiceIntegrationTestSuite) TestParseSceneSlug_MissCacheHoldsUntilExpiry() {
	slug := "cache-me-zz"
	_, _, err := suite.sceneService.ParseSceneSlug(slug)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "scene not found")

	suite.createVerifiedVenue("Later Room", "Cache Me", "ZZ")
	_, _, err = suite.sceneService.ParseSceneSlug(slug)
	suite.Require().Error(err, "cached miss must not resolve a venue that appeared inside the TTL")
	suite.Contains(err.Error(), "scene not found")

	suite.sceneService.missMu.Lock()
	suite.sceneService.slugMisses[strings.ToLower(slug)] = time.Now().Add(-time.Second)
	suite.sceneService.missMu.Unlock()

	city, state, err := suite.sceneService.ParseSceneSlug(slug)
	suite.Require().NoError(err)
	suite.Equal("Cache Me", city)
	suite.Equal("ZZ", state)
}

func (suite *SceneServiceIntegrationTestSuite) TestParseSceneSlug_HitIsNotCachedAsMiss() {
	city, state, err := suite.sceneService.ParseSceneSlug("tempe-az")
	suite.Require().NoError(err)
	suite.Equal("Phoenix", city)
	suite.Equal("AZ", state)

	suite.sceneService.missMu.Lock()
	_, cached := suite.sceneService.slugMisses["tempe-az"]
	suite.sceneService.missMu.Unlock()
	suite.False(cached)
}

func (suite *SceneServiceIntegrationTestSuite) TestParseSceneSlug_UnresolvableUsesIndex() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, err = sqlDB.Exec(`
		INSERT INTO venues (name, city, state, verified, created_at, updated_at)
		SELECT 'Index Venue ' || g, 'IndexCity' || g, 'ZZ', true, NOW(), NOW()
		FROM generate_series(1, 3000) g
	`)
	suite.Require().NoError(err)
	_, err = sqlDB.Exec("ANALYZE venues")
	suite.Require().NoError(err)

	withIndex := explainSceneSlugMiss(suite, "no-such-place-xx")
	suite.Contains(withIndex, "Index Only Scan")
	suite.Contains(withIndex, "idx_venues_verified_scene_slug")
	suite.NotContains(withIndex, "Seq Scan")

	_, err = sqlDB.Exec("DROP INDEX idx_venues_verified_scene_slug")
	suite.Require().NoError(err)
	defer func() {
		_, recErr := sqlDB.Exec(`
			CREATE INDEX idx_venues_verified_scene_slug
			    ON venues (
			        (LOWER(REPLACE(city, ' ', '-')) || '-' || LOWER(state)),
			        city,
			        state
			    )
			    WHERE verified = true
		`)
		suite.Require().NoError(recErr)
	}()
	_, err = sqlDB.Exec("ANALYZE venues")
	suite.Require().NoError(err)
	withoutIndex := explainSceneSlugMiss(suite, "no-such-place-xx")
	suite.Contains(withoutIndex, "Seq Scan")
}

func explainSceneSlugMiss(suite *SceneServiceIntegrationTestSuite, slug string) string {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	rows, err := sqlDB.Query(`
		EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
		SELECT city, state
		FROM venues
		WHERE verified = true
		  AND `+sceneSlugExprSQL+` = $1
		ORDER BY city, state
		LIMIT 1
	`, strings.ToLower(slug))
	suite.Require().NoError(err)
	defer rows.Close() //nolint:errcheck // deferred Close; nothing actionable on failure
	var b strings.Builder
	for rows.Next() {
		var line string
		suite.Require().NoError(rows.Scan(&line))
		b.WriteString(line)
		b.WriteByte('\n')
	}
	suite.Require().NoError(rows.Err())
	return b.String()
}

// =============================================================================
// NormalizedShannonEntropy Unit Tests
// =============================================================================

func TestNormalizedShannonEntropy_Empty(t *testing.T) {
	assert.Equal(t, 0.0, NormalizedShannonEntropy([]int{}))
}

func TestNormalizedShannonEntropy_SingleGenre(t *testing.T) {
	// Only 1 genre => max entropy = log2(1) = 0, so we return 0 (avoid div-by-zero)
	assert.Equal(t, 0.0, NormalizedShannonEntropy([]int{100}))
}

func TestNormalizedShannonEntropy_EqualDistribution(t *testing.T) {
	// Perfectly even distribution of 4 genres => normalized entropy = 1.0
	result := NormalizedShannonEntropy([]int{25, 25, 25, 25})
	assert.InDelta(t, 1.0, result, 0.001)
}

func TestNormalizedShannonEntropy_UnevenDistribution(t *testing.T) {
	// One dominant genre => low entropy
	result := NormalizedShannonEntropy([]int{90, 5, 3, 2})
	assert.Greater(t, result, 0.0)
	assert.Less(t, result, 0.6) // should be low
}

func TestNormalizedShannonEntropy_TwoGenres(t *testing.T) {
	// 50/50 split with 2 genres => normalized entropy = 1.0
	result := NormalizedShannonEntropy([]int{50, 50})
	assert.InDelta(t, 1.0, result, 0.001)
}

func TestNormalizedShannonEntropy_AllZeros(t *testing.T) {
	assert.Equal(t, 0.0, NormalizedShannonEntropy([]int{0, 0, 0}))
}

// =============================================================================
// DiversityLabel Unit Tests
// =============================================================================

func TestDiversityLabel(t *testing.T) {
	tests := []struct {
		index    float64
		expected string
	}{
		{-1, ""},
		{0.1, ""},
		{0.19, ""},
		{0.2, "Genre-focused"},
		{0.4, "Genre-focused"},
		{0.5, "Mixed"},
		{0.7, "Mixed"},
		{0.8, "Highly diverse"},
		{0.95, "Highly diverse"},
		{1.0, "Highly diverse"},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%.2f", tc.index), func(t *testing.T) {
			assert.Equal(t, tc.expected, DiversityLabel(tc.index))
		})
	}
}

// =============================================================================
// Genre Distribution Integration Tests
// =============================================================================

// createGenreTag creates a genre tag for testing
func (suite *SceneServiceIntegrationTestSuite) createGenreTag(name, slug string) uint {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	var tagID uint
	err = sqlDB.QueryRow(`
		INSERT INTO tags (name, slug, category, is_official, usage_count, created_at, updated_at)
		VALUES ($1, $2, 'genre', true, 0, NOW(), NOW())
		RETURNING id
	`, name, slug).Scan(&tagID)
	suite.Require().NoError(err)
	return tagID
}

// tagArtist tags an artist with a genre tag
func (suite *SceneServiceIntegrationTestSuite) tagArtist(artistID, tagID, userID uint) {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, err = sqlDB.Exec(`
		INSERT INTO entity_tags (entity_type, entity_id, tag_id, added_by_user_id, created_at)
		VALUES ('artist', $1, $2, $3, NOW())
	`, artistID, tagID, userID)
	suite.Require().NoError(err)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGenreDistribution_InsufficientData() {
	// Seed scene with 3 venues and 5 shows (3 artists), no tags
	suite.seedSceneData()

	genres, err := suite.sceneService.GetSceneGenreDistribution("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Empty(genres) // No tagged artists at all
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGenreDistribution_BelowThreshold() {
	// Create scene data with a few tagged artists (below 30 threshold)
	venues, artists := suite.seedSceneData()
	_ = venues
	user := suite.createUser()

	punkTag := suite.createGenreTag("punk", "punk")
	suite.tagArtist(artists[0].ID, punkTag, user.ID) // 1 tagged artist, well below 30

	genres, err := suite.sceneService.GetSceneGenreDistribution("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Empty(genres) // Below threshold
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGenreDistribution_Success() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("G-V1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("G-V2", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("G-V3", "Phoenix", "AZ")

	punkTag := suite.createGenreTag("punk", "punk")
	indieTag := suite.createGenreTag("indie rock", "indie-rock")
	metalTag := suite.createGenreTag("metal", "metal")

	future := time.Now().UTC().AddDate(0, 0, 7)

	// Create 35 artists with shows, tag them with genres
	// This ensures we meet the 30 tagged artist threshold
	venues := []*catalogm.Venue{v1, v2, v3}
	tags := []uint{punkTag, punkTag, indieTag, indieTag, indieTag, metalTag}
	for i := 0; i < 35; i++ {
		a := suite.createArtist(fmt.Sprintf("Genre Artist %d", i))
		suite.createApprovedShow(
			fmt.Sprintf("Genre Show %d", i),
			venues[i%3].ID, a.ID, user.ID,
			future.AddDate(0, 0, i),
		)
		tagIdx := i % len(tags)
		suite.tagArtist(a.ID, tags[tagIdx], user.ID)
	}

	genres, err := suite.sceneService.GetSceneGenreDistribution("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.NotEmpty(genres)

	// Should be sorted by count DESC
	suite.GreaterOrEqual(genres[0].Count, genres[len(genres)-1].Count)

	// All genres should have tag_id, name, and slug
	for _, g := range genres {
		suite.NotZero(g.TagID)
		suite.NotEmpty(g.Name)
		suite.NotEmpty(g.Slug)
		suite.Greater(g.Count, 0)
	}
}

// =============================================================================
// Genre Diversity Index Integration Tests
// =============================================================================

func (suite *SceneServiceIntegrationTestSuite) TestGetGenreDiversityIndex_InsufficientArtists() {
	suite.seedSceneData()
	// No tags => insufficient data
	index, err := suite.sceneService.GetGenreDiversityIndex("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Equal(-1.0, index)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetGenreDiversityIndex_InsufficientGenres() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("DI-V1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("DI-V2", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("DI-V3", "Phoenix", "AZ")

	punkTag := suite.createGenreTag("di-punk", "di-punk")

	future := time.Now().UTC().AddDate(0, 0, 7)
	venues := []*catalogm.Venue{v1, v2, v3}

	// 55 artists all tagged with one genre => only 1 genre, below 5 minimum
	for i := 0; i < 55; i++ {
		a := suite.createArtist(fmt.Sprintf("DI Artist %d", i))
		suite.createApprovedShow(
			fmt.Sprintf("DI Show %d", i),
			venues[i%3].ID, a.ID, user.ID,
			future.AddDate(0, 0, i),
		)
		suite.tagArtist(a.ID, punkTag, user.ID)
	}

	index, err := suite.sceneService.GetGenreDiversityIndex("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Equal(-1.0, index) // Insufficient genres (only 1)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetGenreDiversityIndex_Success() {
	user := suite.createUser()
	v1 := suite.createVerifiedVenue("DIX-V1", "Phoenix", "AZ")
	v2 := suite.createVerifiedVenue("DIX-V2", "Phoenix", "AZ")
	v3 := suite.createVerifiedVenue("DIX-V3", "Phoenix", "AZ")

	// Create 6 genres to meet the 5-genre minimum
	genreTags := []uint{
		suite.createGenreTag("dix-punk", "dix-punk"),
		suite.createGenreTag("dix-indie", "dix-indie"),
		suite.createGenreTag("dix-metal", "dix-metal"),
		suite.createGenreTag("dix-jazz", "dix-jazz"),
		suite.createGenreTag("dix-electronic", "dix-electronic"),
		suite.createGenreTag("dix-folk", "dix-folk"),
	}

	future := time.Now().UTC().AddDate(0, 0, 7)
	venues := []*catalogm.Venue{v1, v2, v3}

	// Create 55 artists evenly distributed across genres
	for i := 0; i < 55; i++ {
		a := suite.createArtist(fmt.Sprintf("DIX Artist %d", i))
		suite.createApprovedShow(
			fmt.Sprintf("DIX Show %d", i),
			venues[i%3].ID, a.ID, user.ID,
			future.AddDate(0, 0, i),
		)
		suite.tagArtist(a.ID, genreTags[i%len(genreTags)], user.ID)
	}

	index, err := suite.sceneService.GetGenreDiversityIndex("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Greater(index, 0.0)
	suite.LessOrEqual(index, 1.0)
	// With nearly even distribution across 6 genres, expect high diversity
	suite.Greater(index, 0.8)
}

// TestListScenes_DominantGenre exercises the full ListScenes -> dominant_genre
// wiring end-to-end (the batched sceneGenreCounts query + the scene-key match + the
// family rollup). The pure dominantGenreFamily unit tests don't touch the SQL or
// the key derivation, so a silent metro/city|state key mismatch would leave every
// dot untinted with nothing failing — this guards that. Phoenix's roster is
// punk-dominant, so its dot tints.
func (suite *SceneServiceIntegrationTestSuite) TestListScenes_DominantGenre() {
	suite.seedSceneData() // Phoenix qualifies as a scene (3 venues, 5 shows)
	user := suite.createUser()
	punk := suite.createGenreTag("punk", "punk")
	electronic := suite.createGenreTag("electronic", "electronic")

	// 5 punk + 2 electronic Phoenix artists -> punk 5/7 = 71% (>= 40%), total 7
	// (>= the tagged-artist floor). The 3 untagged seedSceneData artists don't count.
	for i := 0; i < 5; i++ {
		a := suite.createArtist(fmt.Sprintf("Punk Band %d", i))
		suite.tagArtist(a.ID, punk, user.ID)
	}
	for i := 0; i < 2; i++ {
		a := suite.createArtist(fmt.Sprintf("Electro Band %d", i))
		suite.tagArtist(a.ID, electronic, user.ID)
	}

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)

	found := false
	for _, sc := range scenes {
		if sc.City == "Phoenix" && sc.State == "AZ" {
			suite.Equal("punk_hardcore", sc.DominantGenre)
			found = true
		}
	}
	suite.Require().True(found, "Phoenix scene should be present in ListScenes")
}

// TestListScenes_DominantGenre_NeutralWhenMixed pins the orange-stays-orange path:
// a scene with no family clearing the >=40% confidence threshold emits "".
func (suite *SceneServiceIntegrationTestSuite) TestListScenes_DominantGenre_NeutralWhenMixed() {
	suite.seedSceneData()
	user := suite.createUser()
	punk := suite.createGenreTag("punk", "punk")
	electronic := suite.createGenreTag("electronic", "electronic")
	folk := suite.createGenreTag("folk", "folk")

	// 2 punk + 2 electronic + 2 folk -> each family 2/6 = 33% (< 40%), total 6
	// (>= floor, so this exercises the <40% path, not the floor). No confident
	// dominant family, so the dot stays neutral.
	tags := []uint{punk, punk, electronic, electronic, folk, folk}
	for i, tag := range tags {
		a := suite.createArtist(fmt.Sprintf("Mixed Band %d", i))
		suite.tagArtist(a.ID, tag, user.ID)
	}

	scenes, err := suite.sceneService.ListScenes()
	suite.Require().NoError(err)

	found := false
	for _, sc := range scenes {
		if sc.City == "Phoenix" && sc.State == "AZ" {
			suite.Equal("", sc.DominantGenre, "mixed scene must stay neutral")
			found = true
		}
	}
	suite.Require().True(found, "Phoenix scene should be present")
}
