package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/testutil"
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
	}
	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, buildSceneSlug(tc.city, tc.state))
		})
	}
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
	suite.Equal(5, scenes[0].UpcomingShowCount, "this-week shows are also upcoming")
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
