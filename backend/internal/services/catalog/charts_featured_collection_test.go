package catalog

// PSY-1500: public reads over collection_feature_runs — the Broadsheet live
// pick (most recently featured among open runs) and the paginated archive
// (open + closed, newest-first, with featured_at_estimated on the wire).
// Methods hang off ChartsServiceIntegrationTestSuite (charts_service_test.go).

import (
	"time"

	communitym "psychic-homily-backend/internal/models/community"
)

func (suite *ChartsServiceIntegrationTestSuite) createCollection(creatorID uint, title, slug string) *communitym.Collection {
	c := &communitym.Collection{
		Title:     title,
		Slug:      slug,
		CreatorID: creatorID,
		IsPublic:  true,
	}
	suite.Require().NoError(suite.db.Create(c).Error)
	return c
}

// insertRun writes a feature run directly so tests control featured_at ordering
// and the open/closed shape without depending on the write-side service.
func (suite *ChartsServiceIntegrationTestSuite) insertRun(collectionID uint, featuredAt time.Time, unfeaturedAt *time.Time, estimated bool) *communitym.CollectionFeatureRun {
	run := &communitym.CollectionFeatureRun{
		CollectionID:        collectionID,
		FeaturedAt:          featuredAt,
		UnfeaturedAt:        unfeaturedAt,
		FeaturedAtEstimated: estimated,
	}
	suite.Require().NoError(suite.db.Create(run).Error)
	return run
}

// subscribe adds a distinct subscriber to a collection so subscriber_count
// enrichment on the featured-run reads is exercised.
func (suite *ChartsServiceIntegrationTestSuite) subscribe(collectionID, userID uint) {
	suite.Require().NoError(suite.db.Create(&communitym.CollectionSubscriber{
		CollectionID: collectionID,
		UserID:       userID,
	}).Error)
}

// Live pick = the open run with the newest featured_at, even when several
// collections are featured at once (PSY-1411's "most recently featured" lock).
func (suite *ChartsServiceIntegrationTestSuite) TestGetFeaturedCollection_ReturnsMostRecentlyFeaturedOpenRun() {
	user := suite.createUser("featpick@test.com")
	older := suite.createCollection(user.ID, "Older Pick", "older-pick")
	newer := suite.createCollection(user.ID, "Newer Pick", "newer-pick")

	now := time.Now().UTC()
	suite.insertRun(older.ID, now.Add(-72*time.Hour), nil, false)
	suite.insertRun(newer.ID, now.Add(-1*time.Hour), nil, false)
	// Two distinct subscribers on the winning pick so subscriber_count is
	// enriched (and not just defaulted to zero).
	sub1 := suite.createUser("featpick-sub1@test.com")
	sub2 := suite.createUser("featpick-sub2@test.com")
	suite.subscribe(newer.ID, sub1.ID)
	suite.subscribe(newer.ID, sub2.ID)
	// A closed run that is newer than both must NOT win the LIVE pick.
	closedNewer := suite.createCollection(user.ID, "Closed Newer", "closed-newer")
	suite.insertRun(closedNewer.ID, now, featTimePtr(now.Add(30*time.Minute)), false)

	pick, err := suite.chartsService.GetFeaturedCollection()
	suite.Require().NoError(err)
	suite.Require().NotNil(pick)
	suite.Equal(newer.ID, pick.CollectionID)
	suite.Equal("Newer Pick", pick.Title)
	suite.Nil(pick.UnfeaturedAt)
	suite.Equal(2, pick.SubscriberCount)
}

// Nothing featured → nil, not an error (the FE renders no card).
func (suite *ChartsServiceIntegrationTestSuite) TestGetFeaturedCollection_NilWhenNothingOpen() {
	user := suite.createUser("nofeat@test.com")
	coll := suite.createCollection(user.ID, "Once Featured", "once-featured")
	// Only a CLOSED run exists.
	suite.insertRun(coll.ID, time.Now().Add(-10*time.Hour).UTC(), featTimePtr(time.Now().Add(-5*time.Hour).UTC()), false)

	pick, err := suite.chartsService.GetFeaturedCollection()
	suite.Require().NoError(err)
	suite.Nil(pick)
}

// Archive returns open + closed runs newest-first, paginated, with
// featured_at_estimated surfaced on the wire and item counts enriched.
func (suite *ChartsServiceIntegrationTestSuite) TestGetFeaturedCollectionHistory_NewestFirstPaginatedWithEstimated() {
	user := suite.createUser("archive@test.com")
	c1 := suite.createCollection(user.ID, "Run One", "run-one")
	c2 := suite.createCollection(user.ID, "Run Two", "run-two")
	c3 := suite.createCollection(user.ID, "Run Three", "run-three")

	// Give c3 two items so item_count enrichment is exercised.
	suite.Require().NoError(suite.db.Create(&communitym.CollectionItem{CollectionID: c3.ID, EntityType: communitym.CollectionEntityArtist, EntityID: 1, AddedByUserID: user.ID}).Error)
	suite.Require().NoError(suite.db.Create(&communitym.CollectionItem{CollectionID: c3.ID, EntityType: communitym.CollectionEntityArtist, EntityID: 2, AddedByUserID: user.ID}).Error)

	now := time.Now().UTC()
	suite.insertRun(c1.ID, now.Add(-100*time.Hour), featTimePtr(now.Add(-90*time.Hour)), true) // oldest, closed, estimated
	suite.insertRun(c2.ID, now.Add(-50*time.Hour), featTimePtr(now.Add(-40*time.Hour)), false) // middle, closed
	suite.insertRun(c3.ID, now.Add(-1*time.Hour), nil, false)                          // newest, open

	// Page 1 (limit 2): newest two, newest-first.
	page1, total, err := suite.chartsService.GetFeaturedCollectionHistory(2, 0)
	suite.Require().NoError(err)
	suite.Equal(3, total)
	suite.Require().Len(page1, 2)
	suite.Equal(c3.ID, page1[0].CollectionID)
	suite.Nil(page1[0].UnfeaturedAt)
	suite.Equal(2, page1[0].ItemCount)
	suite.Equal(c2.ID, page1[1].CollectionID)
	suite.Require().NotNil(page1[1].UnfeaturedAt)

	// Page 2 (offset 2): the oldest, estimated run.
	page2, total2, err := suite.chartsService.GetFeaturedCollectionHistory(2, 2)
	suite.Require().NoError(err)
	suite.Equal(3, total2)
	suite.Require().Len(page2, 1)
	suite.Equal(c1.ID, page2[0].CollectionID)
	suite.True(page2[0].FeaturedAtEstimated)
}

// A private collection that gets featured must NEVER leak through the public
// reads — neither the live pick nor the archive (nor the archive total).
func (suite *ChartsServiceIntegrationTestSuite) TestFeaturedReads_ExcludePrivateCollections() {
	user := suite.createUser("private-feat@test.com")
	private := &communitym.Collection{
		Title:     "Secret Pick",
		Slug:      "secret-pick",
		CreatorID: user.ID,
		IsPublic:  false,
	}
	suite.Require().NoError(suite.db.Create(private).Error)
	// GORM omits a false bool on Create (is_public has default:true), so force
	// it off explicitly, then confirm it actually persisted as private.
	suite.Require().NoError(suite.db.Model(private).Update("is_public", false).Error)
	var isPublic bool
	suite.Require().NoError(suite.db.Table("collections").Select("is_public").Where("id = ?", private.ID).Scan(&isPublic).Error)
	suite.Require().False(isPublic, "test setup: collection must be persisted as private")
	suite.insertRun(private.ID, time.Now().UTC(), nil, false)

	pick, err := suite.chartsService.GetFeaturedCollection()
	suite.Require().NoError(err)
	suite.Nil(pick, "private featured collection must not surface as the live pick")

	runs, total, err := suite.chartsService.GetFeaturedCollectionHistory(20, 0)
	suite.Require().NoError(err)
	suite.Equal(0, total, "private featured runs must not count toward the archive total")
	suite.Empty(runs)
}

// Empty archive → empty slice + zero total, no error.
func (suite *ChartsServiceIntegrationTestSuite) TestGetFeaturedCollectionHistory_Empty() {
	runs, total, err := suite.chartsService.GetFeaturedCollectionHistory(20, 0)
	suite.Require().NoError(err)
	suite.Equal(0, total)
	suite.Empty(runs)
}

func featTimePtr(t time.Time) *time.Time { return &t }
