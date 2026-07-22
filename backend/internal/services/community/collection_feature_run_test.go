package community

// PSY-1500: transactional SetFeatured + collection_feature_runs journal. These
// exercise the invariant that collections.is_featured and the run journal never
// drift, that re-featuring retains history, and that the admin list folds in the
// open run's featured_at. The public read paths (live pick + archive) are tested
// in the catalog charts suite; the migration/backfill/index in the migration
// suite. Methods hang off CollectionServiceIntegrationTestSuite (see
// collection_test.go) to reuse its DB + fixtures.

import (
	"time"

	"gorm.io/gorm"

	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

func (suite *CollectionServiceIntegrationTestSuite) openRunsFor(collectionID uint) []communitym.CollectionFeatureRun {
	var runs []communitym.CollectionFeatureRun
	err := suite.db.Where("collection_id = ? AND unfeatured_at IS NULL", collectionID).Find(&runs).Error
	suite.Require().NoError(err)
	return runs
}

func (suite *CollectionServiceIntegrationTestSuite) allRunsFor(collectionID uint) []communitym.CollectionFeatureRun {
	var runs []communitym.CollectionFeatureRun
	err := suite.db.Where("collection_id = ?", collectionID).Order("featured_at ASC, id ASC").Find(&runs).Error
	suite.Require().NoError(err)
	return runs
}

// Featuring flips the boolean AND opens exactly one run stamped with the actor.
func (suite *CollectionServiceIntegrationTestSuite) TestSetFeatured_OpensRunAndRecordsActor() {
	user := suite.createTestUser("Featurer")
	coll := suite.createBasicCollection(user, "Open A Run")

	suite.Require().NoError(suite.collectionService.SetFeatured(coll.Slug, true, user.ID))

	detail, err := suite.collectionService.GetBySlug(coll.Slug, user.ID)
	suite.Require().NoError(err)
	suite.True(detail.IsFeatured)

	runs := suite.openRunsFor(coll.ID)
	suite.Require().Len(runs, 1)
	suite.Nil(runs[0].UnfeaturedAt)
	suite.False(runs[0].FeaturedAtEstimated)
	suite.Require().NotNil(runs[0].FeaturedBy)
	suite.Equal(user.ID, *runs[0].FeaturedBy)
	suite.False(runs[0].FeaturedAt.IsZero())
}

// Featuring an already-featured collection is a no-op — no second open run
// (the boolean and the journal stay in agreement).
func (suite *CollectionServiceIntegrationTestSuite) TestSetFeatured_FeatureTwiceIsIdempotent() {
	user := suite.createTestUser("DoubleFeaturer")
	coll := suite.createBasicCollection(user, "Feature Twice")

	suite.Require().NoError(suite.collectionService.SetFeatured(coll.Slug, true, user.ID))
	suite.Require().NoError(suite.collectionService.SetFeatured(coll.Slug, true, user.ID))

	suite.Len(suite.openRunsFor(coll.ID), 1)
	suite.Len(suite.allRunsFor(coll.ID), 1)
}

// Unfeaturing flips the boolean AND closes the open run with the actor.
func (suite *CollectionServiceIntegrationTestSuite) TestSetFeatured_UnfeatureClosesRun() {
	user := suite.createTestUser("Closer")
	coll := suite.createBasicCollection(user, "Close The Run")

	suite.Require().NoError(suite.collectionService.SetFeatured(coll.Slug, true, user.ID))
	suite.Require().NoError(suite.collectionService.SetFeatured(coll.Slug, false, user.ID))

	detail, err := suite.collectionService.GetBySlug(coll.Slug, user.ID)
	suite.Require().NoError(err)
	suite.False(detail.IsFeatured)

	suite.Empty(suite.openRunsFor(coll.ID))
	all := suite.allRunsFor(coll.ID)
	suite.Require().Len(all, 1)
	suite.Require().NotNil(all[0].UnfeaturedAt)
	suite.Require().NotNil(all[0].UnfeaturedBy)
	suite.Equal(user.ID, *all[0].UnfeaturedBy)
}

// Unfeaturing when nothing is open closes nothing and does not error.
func (suite *CollectionServiceIntegrationTestSuite) TestSetFeatured_UnfeatureWhenNotFeaturedIsNoop() {
	user := suite.createTestUser("NoopCloser")
	coll := suite.createBasicCollection(user, "Never Featured")

	suite.Require().NoError(suite.collectionService.SetFeatured(coll.Slug, false, user.ID))

	suite.Empty(suite.allRunsFor(coll.ID))
}

// Re-featuring after an unfeature opens a NEW run: history is retained as two
// rows (one closed, one open), never overwritten.
func (suite *CollectionServiceIntegrationTestSuite) TestSetFeatured_RefeatureCreatesNewRunRetainingHistory() {
	user := suite.createTestUser("Refeaturer")
	coll := suite.createBasicCollection(user, "Re-feature Me")

	suite.Require().NoError(suite.collectionService.SetFeatured(coll.Slug, true, user.ID))
	suite.Require().NoError(suite.collectionService.SetFeatured(coll.Slug, false, user.ID))
	suite.Require().NoError(suite.collectionService.SetFeatured(coll.Slug, true, user.ID))

	all := suite.allRunsFor(coll.ID)
	suite.Require().Len(all, 2)
	// Oldest run closed, newest run open.
	suite.NotNil(all[0].UnfeaturedAt)
	suite.Nil(all[1].UnfeaturedAt)
	suite.Len(suite.openRunsFor(coll.ID), 1)
}

// The partial unique index rejects a SECOND open run for the same collection —
// the single invariant that makes "most recently featured among open runs"
// well-defined. A raw insert bypasses SetFeatured's idempotency guard to prove
// the DB-level constraint, not just the service logic.
func (suite *CollectionServiceIntegrationTestSuite) TestFeatureRuns_PartialUniqueIndexRejectsSecondOpenRun() {
	user := suite.createTestUser("IndexProver")
	coll := suite.createBasicCollection(user, "One Open Only")

	suite.Require().NoError(suite.collectionService.SetFeatured(coll.Slug, true, user.ID))

	second := &communitym.CollectionFeatureRun{
		CollectionID: coll.ID,
		FeaturedAt:   time.Now().UTC(),
	}
	err := suite.db.Create(second).Error
	suite.Require().Error(err)
	suite.ErrorIs(err, gorm.ErrDuplicatedKey)

	// A CLOSED run alongside the open one is legal (re-featuring depends on it).
	closed := &communitym.CollectionFeatureRun{
		CollectionID: coll.ID,
		FeaturedAt:   time.Now().Add(-48 * time.Hour).UTC(),
		UnfeaturedAt: ptrTime(time.Now().Add(-24 * time.Hour).UTC()),
	}
	suite.Require().NoError(suite.db.Create(closed).Error)
}

// The admin/browse list folds the open run's featured_at + estimated flag into
// the payload (PSY-1504 reads this, no second per-row fetch). Unfeatured rows
// carry nil.
func (suite *CollectionServiceIntegrationTestSuite) TestListCollections_FoldsOpenRunFeaturedAt() {
	user := suite.createTestUser("AdminLister")
	featured := suite.createBasicCollection(user, "Featured Row")
	suite.createBasicCollection(user, "Plain Row")

	suite.Require().NoError(suite.collectionService.SetFeatured(featured.Slug, true, user.ID))

	resp, _, err := suite.collectionService.ListCollections(contracts.CollectionFilters{CreatorID: user.ID}, 20, 0)
	suite.Require().NoError(err)

	var seenFeatured, seenPlain bool
	for _, c := range resp {
		if c.ID == featured.ID {
			seenFeatured = true
			suite.Require().NotNil(c.FeaturedAt)
			suite.False(c.FeaturedAt.IsZero())
			suite.Require().NotNil(c.FeaturedAtEstimated)
			suite.False(*c.FeaturedAtEstimated)
		} else {
			seenPlain = true
			suite.Nil(c.FeaturedAt)
			suite.Nil(c.FeaturedAtEstimated)
		}
	}
	suite.True(seenFeatured)
	suite.True(seenPlain)
}

func ptrTime(t time.Time) *time.Time { return &t }
