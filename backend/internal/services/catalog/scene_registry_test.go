package catalog

import (
	"time"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/engagement"
)

// Scene registry tests (PSY-1339) — run as part of the
// SceneServiceIntegrationTestSuite (real Postgres, all migrations).
// Registry rows and follows are cleaned per-test in TearDownTest.

func (suite *SceneServiceIntegrationTestSuite) TestGetOrCreateSceneID_MetroCanonicalization() {
	suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")

	id, err := suite.sceneService.GetOrCreateSceneID("phoenix-az")
	suite.Require().NoError(err)
	suite.Require().NotZero(id)

	// Idempotent: same slug → same row.
	again, err := suite.sceneService.GetOrCreateSceneID("phoenix-az")
	suite.Require().NoError(err)
	suite.Equal(id, again)

	// A metro MEMBER city's slug canonicalizes to the SAME row — following
	// Mesa follows the Phoenix scene (the PSY-1255 metro roster model).
	viaMember, err := suite.sceneService.GetOrCreateSceneID("mesa-az")
	suite.Require().NoError(err)
	suite.Equal(id, viaMember)

	var row catalogm.Scene
	suite.Require().NoError(suite.db.First(&row, id).Error)
	suite.Require().NotNil(row.Metro)
	suite.Equal("38060", *row.Metro) // Phoenix-Mesa-Chandler CBSA
	suite.Equal("phoenix-az", row.Slug)

	var count int64
	suite.db.Model(&catalogm.Scene{}).Count(&count)
	suite.Equal(int64(1), count, "canonicalization must not create a second row")
}

func (suite *SceneServiceIntegrationTestSuite) TestGetOrCreateSceneID_UnknownSlugIsNotFound() {
	_, err := suite.sceneService.GetOrCreateSceneID("nowhere-zz")
	suite.Require().Error(err)

	var count int64
	suite.db.Model(&catalogm.Scene{}).Count(&count)
	suite.Zero(count, "a failed resolution must not materialize a row")
}

func (suite *SceneServiceIntegrationTestSuite) TestGetOrCreateSceneID_FallbackScene() {
	// A city the US-only geocoder can't pin to a CBSA falls back to the
	// literal (city, state) scope — resolved via its verified venue.
	suite.createVerifiedVenue("SO36", "Berlin", "DE")

	id, err := suite.sceneService.GetOrCreateSceneID("berlin-de")
	suite.Require().NoError(err)

	var row catalogm.Scene
	suite.Require().NoError(suite.db.First(&row, id).Error)
	suite.Nil(row.Metro)
	suite.Equal("Berlin", row.City)
	suite.Equal("berlin-de", row.Slug)
}

func (suite *SceneServiceIntegrationTestSuite) TestLookupSceneID_DoesNotCreate() {
	suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")

	_, ok, err := suite.sceneService.LookupSceneID("phoenix-az")
	suite.Require().NoError(err)
	suite.False(ok, "no row until something needs one")

	var count int64
	suite.db.Model(&catalogm.Scene{}).Count(&count)
	suite.Zero(count)
}

func (suite *SceneServiceIntegrationTestSuite) TestSceneFollow_CountsAndFollowingHydration() {
	suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")
	user := &authm.User{}
	suite.Require().NoError(suite.db.Create(user).Error)

	sceneID, err := suite.sceneService.GetOrCreateSceneID("phoenix-az")
	suite.Require().NoError(err)

	follows := engagement.NewFollowService(suite.db)
	suite.Require().NoError(follows.Follow(user.ID, "scene", sceneID))

	count, err := follows.GetFollowerCount("scene", sceneID)
	suite.Require().NoError(err)
	suite.Equal(int64(1), count)

	following, total, err := follows.GetUserFollowing(user.ID, "scene", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(following, 1)
	suite.Equal("scene", following[0].EntityType)
	suite.Equal(sceneID, following[0].EntityID)
	suite.Equal("Phoenix, AZ", following[0].Name)
	suite.Equal("phoenix-az", following[0].Slug)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneDetail_DescriptionFromRegistry() {
	suite.seedSceneData() // detail requires the scene to exist (venues + shows)

	// No registry row → nil description (the pre-registry behavior).
	detail, err := suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Nil(detail.Description)

	id, err := suite.sceneService.GetOrCreateSceneID("phoenix-az")
	suite.Require().NoError(err)
	suite.Require().NoError(
		suite.db.Model(&catalogm.Scene{}).Where("id = ?", id).
			Update("description", "Desert DIY forever.").Error)

	detail, err = suite.sceneService.GetSceneDetail("Phoenix", "AZ")
	suite.Require().NoError(err)
	suite.Require().NotNil(detail.Description)
	suite.Equal("Desert DIY forever.", *detail.Description)
}

func (suite *SceneServiceIntegrationTestSuite) TestGetOrCreateSceneID_UpgradesFallbackSquatterInPlace() {
	suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")

	// Simulate the drift case: a fallback row created before the geocoder
	// could resolve the metro now squats the canonical slug.
	squatter := &catalogm.Scene{City: "Phoenix", State: "AZ", Slug: "phoenix-az"}
	suite.Require().NoError(suite.db.Create(squatter).Error)

	id, err := suite.sceneService.GetOrCreateSceneID("phoenix-az")
	suite.Require().NoError(err)
	suite.Equal(squatter.ID, id, "must converge on the existing row, not error or duplicate")

	var row catalogm.Scene
	suite.Require().NoError(suite.db.First(&row, id).Error)
	suite.Require().NotNil(row.Metro, "the squatting fallback row must be upgraded in place")
	suite.Equal("38060", *row.Metro)

	var count int64
	suite.db.Model(&catalogm.Scene{}).Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *SceneServiceIntegrationTestSuite) TestSceneNotifyMode_RoundTrip() {
	suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")
	user := &authm.User{}
	suite.Require().NoError(suite.db.Create(user).Error)
	sceneID, err := suite.sceneService.GetOrCreateSceneID("phoenix-az")
	suite.Require().NoError(err)

	follows := engagement.NewFollowService(suite.db)

	// Configuring a mode without a follow is an error, not a silent no-op.
	suite.Error(follows.SetSceneNotifyMode(user.ID, sceneID, engagement.SceneNotifyModeFollowedBands))

	suite.Require().NoError(follows.Follow(user.ID, "scene", sceneID))

	// Default (absent settings) reads as "all".
	mode, err := follows.SceneNotifyMode(user.ID, sceneID)
	suite.Require().NoError(err)
	suite.Equal(engagement.SceneNotifyModeAll, mode)

	suite.Require().NoError(follows.SetSceneNotifyMode(user.ID, sceneID, engagement.SceneNotifyModeFollowedBands))
	mode, err = follows.SceneNotifyMode(user.ID, sceneID)
	suite.Require().NoError(err)
	suite.Equal(engagement.SceneNotifyModeFollowedBands, mode)

	// Invalid mode rejected.
	suite.Error(follows.SetSceneNotifyMode(user.ID, sceneID, "hourly"))
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneNewArtistsSince() {
	// A fallback scene (ZZ has no CBSA → city/state scope, no geocoder dep).
	since := time.Now().Add(-48 * time.Hour)
	mk := func(name string, createdAt time.Time) {
		slug := name
		a := catalogm.Artist{Name: name, Slug: &slug, City: ptr("Testville"), State: ptr("ZZ"), CreatedAt: createdAt, UpdatedAt: createdAt}
		suite.Require().NoError(suite.db.Create(&a).Error)
	}
	mk("Fresh Band", time.Now().Add(-12*time.Hour)) // after `since` → included
	mk("Stale Band", time.Now().Add(-96*time.Hour)) // before `since` → excluded
	// An artist based elsewhere must not leak in.
	other := "Other Town Band"
	suite.Require().NoError(suite.db.Create(&catalogm.Artist{Name: other, Slug: &other, City: ptr("Elsewhere"), State: ptr("ZZ"), CreatedAt: time.Now(), UpdatedAt: time.Now()}).Error)

	out, total, err := suite.sceneService.GetSceneNewArtistsSince("Testville", "ZZ", since, time.Now(), 10)
	suite.Require().NoError(err)
	suite.Require().Len(out, 1)
	suite.Equal(1, total)
	suite.Equal("Fresh Band", out[0].Name)

	// Cap smaller than the window → the list is capped but total still counts all.
	capped, total2, err := suite.sceneService.GetSceneNewArtistsSince("Testville", "ZZ", since, time.Now(), 0)
	suite.Require().NoError(err)
	suite.Len(capped, 0)
	suite.Equal(1, total2)
}

func ptr(s string) *string { return &s }
