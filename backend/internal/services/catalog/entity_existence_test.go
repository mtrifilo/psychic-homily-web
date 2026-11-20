package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/testutil"
)

type EntityExistenceServiceIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *EntityExistenceService
}

func (suite *EntityExistenceServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.svc = NewEntityExistenceService(suite.db, NewSceneService(suite.db))
}

func (suite *EntityExistenceServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *EntityExistenceServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM radio_episodes")
	_, _ = sqlDB.Exec("DELETE FROM radio_shows")
	_, _ = sqlDB.Exec("DELETE FROM radio_stations")
	_, _ = sqlDB.Exec("DELETE FROM collection_subscribers")
	_, _ = sqlDB.Exec("DELETE FROM collections")
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
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestEntityExistenceServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(EntityExistenceServiceIntegrationTestSuite))
}

func (suite *EntityExistenceServiceIntegrationTestSuite) createEntityExistenceUser() *authm.User {
	user := &authm.User{
		Email:         stringPtr(fmt.Sprintf("entity-existence-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Entity"),
		LastName:      stringPtr("Probe"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(user).Error)
	return user
}

func (suite *EntityExistenceServiceIntegrationTestSuite) TestExists_ShowSlugUsesPublicDetailSemantics() {
	user := suite.createEntityExistenceUser()
	approvedSlug := "approved-show"
	privateSlug := "private-show"

	approved := &catalogm.Show{
		Title:       "Approved Show",
		Slug:        &approvedSlug,
		EventDate:   time.Now().UTC().Add(24 * time.Hour),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	private := &catalogm.Show{
		Title:       "Private Show",
		Slug:        &privateSlug,
		EventDate:   time.Now().UTC().Add(48 * time.Hour),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusPrivate,
		SubmittedBy: &user.ID,
	}
	suite.Require().NoError(suite.db.Create(approved).Error)
	suite.Require().NoError(suite.db.Create(private).Error)

	exists, err := suite.svc.Exists("shows", approvedSlug)
	suite.Require().NoError(err)
	suite.True(exists)

	exists, err = suite.svc.Exists("shows", fmt.Sprintf("%d", approved.ID))
	suite.Require().NoError(err)
	suite.True(exists)

	exists, err = suite.svc.Exists("shows", privateSlug)
	suite.Require().NoError(err)
	suite.False(exists)

	exists, err = suite.svc.Exists("shows", "missing-show")
	suite.Require().NoError(err)
	suite.False(exists)
}

func (suite *EntityExistenceServiceIntegrationTestSuite) TestExists_TagSlugAndID() {
	tag := &catalogm.Tag{Name: "Post Punk", Slug: "post-punk", Category: catalogm.TagCategoryGenre}
	suite.Require().NoError(suite.db.Create(tag).Error)

	exists, err := suite.svc.Exists("tags", "post-punk")
	suite.Require().NoError(err)
	suite.True(exists)

	exists, err = suite.svc.Exists("tags", fmt.Sprintf("%d", tag.ID))
	suite.Require().NoError(err)
	suite.True(exists)

	exists, err = suite.svc.Exists("tags", "missing-tag")
	suite.Require().NoError(err)
	suite.False(exists)
}

func (suite *EntityExistenceServiceIntegrationTestSuite) TestExists_SceneSlugUsesDetailThreshold() {
	// Two verified venues in the Phoenix metro (Phoenix + Mesa share CBSA 38060)
	// and one in the SEPARATE Tucson metro. Metro is denormalized as the
	// production write paths do (PSY-1255 step C).
	venues := []*catalogm.Venue{
		{Name: "Crescent Ballroom", Slug: stringPtr("crescent-ballroom"), City: "Phoenix", State: "AZ", Metro: seedMetro("Phoenix", "AZ"), Verified: true},
		{Name: "Valley Bar", Slug: stringPtr("valley-bar"), City: "Phoenix", State: "AZ", Metro: seedMetro("Phoenix", "AZ"), Verified: true},
		{Name: "Small Room", Slug: stringPtr("small-room"), City: "Tucson", State: "AZ", Metro: seedMetro("Tucson", "AZ"), Verified: true},
		{Name: "Unverified Room", Slug: stringPtr("unverified-room"), City: "Mesa", State: "AZ", Metro: seedMetro("Mesa", "AZ"), Verified: false},
	}
	for _, venue := range venues {
		suite.Require().NoError(suite.db.Create(venue).Error)
	}
	suite.Require().NoError(suite.db.Model(venues[3]).Update("verified", false).Error)

	// Phoenix metro clears the threshold (2 verified) → its scene exists.
	exists, err := suite.svc.Exists("scenes", "phoenix-az")
	suite.Require().NoError(err)
	suite.True(exists)

	// A suburb slug in the SAME metro rolls up to it (Mesa → Phoenix metro), so it
	// also resolves — the step-C rollup that keeps old member URLs from 404ing.
	exists, err = suite.svc.Exists("scenes", "mesa-az")
	suite.Require().NoError(err)
	suite.True(exists)

	// Tucson is a separate metro with only 1 verified venue → below threshold.
	exists, err = suite.svc.Exists("scenes", "tucson-az")
	suite.Require().NoError(err)
	suite.False(exists)
}

// TestExists_SceneSlugHyphenatedNoCBSACity guards the adversarial round-2 fix: a
// no-CBSA city whose NAME contains a hyphen ("Foo-Bar") must still pass the
// existence probe. The fallback must match the slug form losslessly (LOWER +
// REPLACE), not re-parse the slug — re-parsing would collapse "foo-bar" to
// "foo bar" and 404 a scene that both lists and renders.
func (suite *EntityExistenceServiceIntegrationTestSuite) TestExists_SceneSlugHyphenatedNoCBSACity() {
	venues := []*catalogm.Venue{
		{Name: "Hyphen Hall", Slug: stringPtr("hyphen-hall"), City: "Foo-Bar", State: "ZZ", Metro: seedMetro("Foo-Bar", "ZZ"), Verified: true},
		{Name: "Dash Den", Slug: stringPtr("dash-den"), City: "Foo-Bar", State: "ZZ", Metro: seedMetro("Foo-Bar", "ZZ"), Verified: true},
	}
	for _, venue := range venues {
		suite.Require().NoError(suite.db.Create(venue).Error)
	}
	suite.Require().Nil(venues[0].Metro, "Foo-Bar is not in any CBSA → fallback path")

	exists, err := suite.svc.Exists("scenes", "foo-bar-zz")
	suite.Require().NoError(err)
	suite.True(exists)
}

// The scene probe names the slug the scene's page is served at: the metro's
// principal for a member city, the lowercase spelling for a case variant, and
// the request itself when it is already canonical. A missing or below-threshold
// scene names none.
func (suite *EntityExistenceServiceIntegrationTestSuite) TestSceneCanonicalSlug() {
	venues := []*catalogm.Venue{
		{Name: "Crescent Ballroom", Slug: stringPtr("crescent-ballroom"), City: "Phoenix", State: "AZ", Metro: seedMetro("Phoenix", "AZ"), Verified: true},
		{Name: "Valley Bar", Slug: stringPtr("valley-bar"), City: "Phoenix", State: "AZ", Metro: seedMetro("Phoenix", "AZ"), Verified: true},
		{Name: "Small Room", Slug: stringPtr("small-room"), City: "Tucson", State: "AZ", Metro: seedMetro("Tucson", "AZ"), Verified: true},
		{Name: "Hyphen Hall", Slug: stringPtr("hyphen-hall"), City: "Foo-Bar", State: "ZZ", Metro: seedMetro("Foo-Bar", "ZZ"), Verified: true},
		{Name: "Dash Den", Slug: stringPtr("dash-den"), City: "Foo-Bar", State: "ZZ", Metro: seedMetro("Foo-Bar", "ZZ"), Verified: true},
	}
	for _, venue := range venues {
		suite.Require().NoError(suite.db.Create(venue).Error)
	}

	cases := []struct {
		requested     string
		wantCanonical string
		wantExists    bool
	}{
		{"phoenix-az", "phoenix-az", true},
		{"PHOENIX-AZ", "phoenix-az", true},
		{"mesa-az", "phoenix-az", true},
		{"Mesa-AZ", "phoenix-az", true},
		{"foo-bar-zz", "foo-bar-zz", true},
		{"FOO-BAR-ZZ", "foo-bar-zz", true},
		{"tucson-az", "", false},
		{"nowhere-zz", "", false},
	}
	for _, tc := range cases {
		canonical, exists, err := suite.svc.SceneCanonicalSlug(tc.requested)
		suite.Require().NoError(err, tc.requested)
		suite.Equal(tc.wantExists, exists, tc.requested)
		suite.Equal(tc.wantCanonical, canonical, tc.requested)

		// The verdict is sceneExists' verdict.
		exists, err = suite.svc.Exists("scenes", tc.requested)
		suite.Require().NoError(err, tc.requested)
		suite.Equal(tc.wantExists, exists, tc.requested)
	}
}

// A canonical slug that does not resolve back to itself is never named: the
// scene still exists, and the probe names no slug for it. The slug-miss cache
// is the lever that makes the canonical spelling unresolvable here.
func (suite *EntityExistenceServiceIntegrationTestSuite) TestSceneCanonicalSlug_NamesNoSlugThatDoesNotResolveBack() {
	venues := []*catalogm.Venue{
		{Name: "Crescent Ballroom", Slug: stringPtr("crescent-ballroom"), City: "Phoenix", State: "AZ", Metro: seedMetro("Phoenix", "AZ"), Verified: true},
		{Name: "Valley Bar", Slug: stringPtr("valley-bar"), City: "Phoenix", State: "AZ", Metro: seedMetro("Phoenix", "AZ"), Verified: true},
	}
	for _, venue := range venues {
		suite.Require().NoError(suite.db.Create(venue).Error)
	}
	scenes := NewSceneService(suite.db)
	scenes.rememberSlugMiss("phoenix-az")
	svc := NewEntityExistenceService(suite.db, scenes)

	canonical, exists, err := svc.SceneCanonicalSlug("mesa-az")
	suite.Require().NoError(err)
	suite.True(exists)
	suite.Empty(canonical)
}

func (suite *EntityExistenceServiceIntegrationTestSuite) TestExists_UnsupportedEntityTypeIsMissing() {
	exists, err := suite.svc.Exists("collections", "any")
	suite.Require().NoError(err)
	suite.False(exists)
}

// seedRadioPair creates two stations, a show on the first and one episode of
// it, returning the first station, the other station, and the show.
func (suite *EntityExistenceServiceIntegrationTestSuite) seedRadioPair() (*catalogm.RadioStation, *catalogm.RadioStation, *catalogm.RadioShow) {
	station := &catalogm.RadioStation{Name: "KEXP", Slug: "kexp"}
	other := &catalogm.RadioStation{Name: "WFMU", Slug: "wfmu"}
	suite.Require().NoError(suite.db.Create(station).Error)
	suite.Require().NoError(suite.db.Create(other).Error)
	show := &catalogm.RadioShow{StationID: station.ID, Name: "The Morning Show", Slug: "the-morning-show"}
	suite.Require().NoError(suite.db.Create(show).Error)
	episode := &catalogm.RadioEpisode{ShowID: show.ID, AirDate: "2025-01-15"}
	suite.Require().NoError(suite.db.Create(episode).Error)
	return station, other, show
}

func (suite *EntityExistenceServiceIntegrationTestSuite) TestRadioStationExists_SlugAndID() {
	station, _, _ := suite.seedRadioPair()

	for _, segment := range []string{"kexp", fmt.Sprintf("%d", station.ID)} {
		exists, err := suite.svc.RadioStationExists(segment)
		suite.Require().NoError(err)
		suite.True(exists, segment)
	}
	for _, segment := range []string{"no-such-station", "KEXP", "999999"} {
		exists, err := suite.svc.RadioStationExists(segment)
		suite.Require().NoError(err)
		suite.False(exists, segment)
	}
}

// The show must be addressed under the station that carries it. A real show
// under another real station, or under a station nobody has, is absent.
func (suite *EntityExistenceServiceIntegrationTestSuite) TestRadioShowExists_RequiresItsOwnStation() {
	station, other, show := suite.seedRadioPair()
	stationID := fmt.Sprintf("%d", station.ID)
	showID := fmt.Sprintf("%d", show.ID)

	present := [][2]string{
		{"kexp", "the-morning-show"},
		{stationID, "the-morning-show"},
		{"kexp", showID},
		{stationID, showID},
	}
	for _, pair := range present {
		exists, err := suite.svc.RadioShowExists(pair[0], pair[1])
		suite.Require().NoError(err)
		suite.True(exists, pair)
	}

	absent := [][2]string{
		{"kexp", "no-such-show"},
		{"wfmu", "the-morning-show"},
		{fmt.Sprintf("%d", other.ID), "the-morning-show"},
		{"no-such-station", "the-morning-show"},
		{"KEXP", "the-morning-show"},
		{"kexp", "The-Morning-Show"},
	}
	for _, pair := range absent {
		exists, err := suite.svc.RadioShowExists(pair[0], pair[1])
		suite.Require().NoError(err)
		suite.False(exists, pair)
	}
}

func (suite *EntityExistenceServiceIntegrationTestSuite) TestRadioEpisodeExists_RequiresTheDayAndTheStation() {
	station, _, _ := suite.seedRadioPair()

	exists, err := suite.svc.RadioEpisodeExists("kexp", "the-morning-show", "2025-01-15")
	suite.Require().NoError(err)
	suite.True(exists)

	exists, err = suite.svc.RadioEpisodeExists(fmt.Sprintf("%d", station.ID), "the-morning-show", "2025-01-15")
	suite.Require().NoError(err)
	suite.True(exists)

	absent := [][3]string{
		{"kexp", "the-morning-show", "2025-01-16"},
		{"wfmu", "the-morning-show", "2025-01-15"},
		{"kexp", "no-such-show", "2025-01-15"},
		{"kexp", "the-morning-show", "2025-02-30"},
		{"kexp", "the-morning-show", "2025-1-15"},
		{"kexp", "the-morning-show", "not-a-date"},
	}
	for _, triple := range absent {
		exists, err := suite.svc.RadioEpisodeExists(triple[0], triple[1], triple[2])
		suite.Require().NoError(err)
		suite.False(exists, triple)
	}
}

// A collection exists to the probe exactly when the detail route serves it to a
// caller without a session: private and closed-creator collections answer as an
// unused slug does, by slug and by numeric id alike.
func (suite *EntityExistenceServiceIntegrationTestSuite) TestCollectionExists_RefusesHiddenCollections() {
	creator := suite.createEntityExistenceUser()
	closedCreator := suite.createEntityExistenceUser()
	suite.Require().NoError(suite.db.Model(closedCreator).Update("is_active", false).Error)

	public := &communitym.Collection{Title: "Public", Slug: "public-list", CreatorID: creator.ID, IsPublic: true, DisplayMode: "unranked"}
	private := &communitym.Collection{Title: "Private", Slug: "private-list", CreatorID: creator.ID, IsPublic: true, DisplayMode: "unranked"}
	closed := &communitym.Collection{Title: "Closed", Slug: "closed-list", CreatorID: closedCreator.ID, IsPublic: true, DisplayMode: "unranked"}
	for _, c := range []*communitym.Collection{public, private, closed} {
		suite.Require().NoError(suite.db.Create(c).Error)
	}
	// Written after the create: GORM skips a false bool whose column has a
	// default, so the create alone would store true.
	suite.Require().NoError(suite.db.Model(private).Update("is_public", false).Error)

	for _, segment := range []string{"public-list", fmt.Sprintf("%d", public.ID)} {
		exists, err := suite.svc.CollectionExists(segment)
		suite.Require().NoError(err)
		suite.True(exists, segment)
	}

	hidden := []string{
		"private-list", fmt.Sprintf("%d", private.ID),
		"closed-list", fmt.Sprintf("%d", closed.ID),
		"no-such-list", "999999",
	}
	for _, segment := range hidden {
		exists, err := suite.svc.CollectionExists(segment)
		suite.Require().NoError(err)
		suite.False(exists, segment)
	}
}
