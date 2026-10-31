package community

import (
	"fmt"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// collectionWriteSnapshot is everything a slug write can change about one
// collection and its neighbourhood, read straight from the tables.
type collectionWriteSnapshot struct {
	Collections int64
	Items       int64
	ItemNotes   string
	Positions   string
	Subscribers int64
	Likes       int64
	Tags        int64
	Featured    bool
	OpenRuns    int64
}

func (suite *CollectionServiceIntegrationTestSuite) snapshotCollectionWrites(collectionID uint) collectionWriteSnapshot {
	var snap collectionWriteSnapshot
	suite.Require().NoError(suite.db.Model(&communitym.Collection{}).Count(&snap.Collections).Error)
	suite.Require().NoError(suite.db.Model(&communitym.CollectionItem{}).
		Where("collection_id = ?", collectionID).Count(&snap.Items).Error)
	var items []communitym.CollectionItem
	suite.Require().NoError(suite.db.Where("collection_id = ?", collectionID).Order("id").Find(&items).Error)
	for _, it := range items {
		notes := ""
		if it.Notes != nil {
			notes = *it.Notes
		}
		snap.ItemNotes += fmt.Sprintf("%d:%s;", it.ID, notes)
		snap.Positions += fmt.Sprintf("%d:%d;", it.ID, it.Position)
	}
	suite.Require().NoError(suite.db.Model(&communitym.CollectionSubscriber{}).
		Where("collection_id = ?", collectionID).Count(&snap.Subscribers).Error)
	suite.Require().NoError(suite.db.Model(&communitym.CollectionLike{}).
		Where("collection_id = ?", collectionID).Count(&snap.Likes).Error)
	suite.Require().NoError(suite.db.Model(&catalogm.EntityTag{}).
		Where("entity_type = ? AND entity_id = ?", catalogm.TagEntityCollection, collectionID).
		Count(&snap.Tags).Error)
	var c communitym.Collection
	suite.Require().NoError(suite.db.Select("is_featured").First(&c, collectionID).Error)
	snap.Featured = c.IsFeatured
	suite.Require().NoError(suite.db.Model(&communitym.CollectionFeatureRun{}).
		Where("collection_id = ? AND unfeatured_at IS NULL", collectionID).Count(&snap.OpenRuns).Error)
	return snap
}

// closedCreatorStates are the two ways a creator's account stops being live.
var closedCreatorStates = []struct {
	name  string
	close func(suite *CollectionServiceIntegrationTestSuite, userID uint)
}{
	{"deactivated", func(suite *CollectionServiceIntegrationTestSuite, userID uint) {
		testutil.DeactivateAccount(suite.T(), suite.db, userID)
	}},
	{"soft-deleted", func(suite *CollectionServiceIntegrationTestSuite, userID uint) {
		testutil.SoftDeleteAccount(suite.T(), suite.db, userID)
	}},
}

// A closed creator's public, collaborative collection refuses every slug write
// a viewer can reach with the not-found answer an unused slug gets, and writes
// nothing: clone, item add (single and bulk), item edit and removal, reorder,
// subscribe, like, tag add and removal, and featuring. Reopening the account
// lets the same writes through again.
func (suite *CollectionServiceIntegrationTestSuite) TestClosedCreator_SlugWritesAnswerNotFoundAndWriteNothing() {
	for i, state := range closedCreatorStates {
		suite.Run(state.name, func() {
			creator := suite.createTestUser(fmt.Sprintf("WriteCreator%d", i))
			suite.promoteContributor(creator)
			viewer := suite.createTestUser(fmt.Sprintf("WriteViewer%d", i))
			suite.promoteContributor(viewer)
			artist := suite.createTestArtist(fmt.Sprintf("Write Artist %d", i))
			other := suite.createTestArtist(fmt.Sprintf("Write Other %d", i))
			c, err := suite.collectionService.CreateCollection(creator.ID, &contracts.CreateCollectionRequest{
				Title: fmt.Sprintf("Closed Writes %d", i), IsPublic: true, Collaborative: true,
			})
			suite.Require().NoError(err)

			viewerItem, _, err := suite.collectionService.AddItem(c.Slug, viewer.ID,
				&contracts.AddCollectionItemRequest{EntityType: "artist", EntityID: artist.ID})
			suite.Require().NoError(err)
			tagged, _, err := suite.collectionService.AddTagToCollection(c.Slug, viewer.ID,
				&contracts.AddCollectionTagRequest{TagName: fmt.Sprintf("closed-writes-%d", i)})
			suite.Require().NoError(err)
			suite.Require().Len(tagged.Tags, 1)
			tagID := tagged.Tags[0].TagID

			state.close(suite, creator.ID)
			before := suite.snapshotCollectionWrites(c.ID)
			notes := "edited after the creator closed"

			_, err = suite.collectionService.CloneCollection(c.Slug, viewer.ID)
			suite.requireCollectionNotFound(err, c.Slug, "CloneCollection")
			_, _, err = suite.collectionService.AddItem(c.Slug, viewer.ID,
				&contracts.AddCollectionItemRequest{EntityType: "artist", EntityID: other.ID})
			suite.requireCollectionNotFound(err, c.Slug, "AddItem")
			_, _, err = suite.collectionService.BulkAddItems(c.Slug, viewer.ID,
				&contracts.BulkAddCollectionItemsRequest{Items: []contracts.AddCollectionItemRequest{
					{EntityType: "artist", EntityID: other.ID},
					{EntityType: "artist", EntityID: artist.ID},
				}})
			suite.requireCollectionNotFound(err, c.Slug, "BulkAddItems")
			_, _, err = suite.collectionService.UpdateItem(c.Slug, viewerItem.ID, viewer.ID, false,
				&contracts.UpdateCollectionItemRequest{Notes: &notes})
			suite.requireCollectionNotFound(err, c.Slug, "UpdateItem")
			_, err = suite.collectionService.RemoveItem(c.Slug, viewerItem.ID, viewer.ID, false)
			suite.requireCollectionNotFound(err, c.Slug, "RemoveItem")
			err = suite.collectionService.ReorderItems(c.Slug, creator.ID,
				&contracts.ReorderCollectionItemsRequest{Items: []contracts.ReorderItem{{ItemID: viewerItem.ID, Position: 9}}})
			suite.requireCollectionNotFound(err, c.Slug, "ReorderItems")
			err = suite.collectionService.Subscribe(c.Slug, viewer.ID)
			suite.requireCollectionNotFound(err, c.Slug, "Subscribe")
			_, err = suite.collectionService.Like(c.Slug, viewer.ID)
			suite.requireCollectionNotFound(err, c.Slug, "Like")
			_, _, err = suite.collectionService.AddTagToCollection(c.Slug, viewer.ID,
				&contracts.AddCollectionTagRequest{TagName: fmt.Sprintf("closed-writes-extra-%d", i)})
			suite.requireCollectionNotFound(err, c.Slug, "AddTagToCollection")
			_, err = suite.collectionService.RemoveTagFromCollection(c.Slug, tagID, viewer.ID)
			suite.requireCollectionNotFound(err, c.Slug, "RemoveTagFromCollection")
			_, err = suite.collectionService.SetFeatured(c.Slug, true, viewer.ID)
			suite.requireCollectionNotFound(err, c.Slug, "SetFeatured(true)")

			suite.Equal(before, suite.snapshotCollectionWrites(c.ID), "no write landed while the creator was closed")

			testutil.ReopenAccount(suite.T(), suite.db, creator.ID)
			_, err = suite.collectionService.CloneCollection(c.Slug, viewer.ID)
			suite.NoError(err, "clone after reopening")
			_, _, err = suite.collectionService.AddItem(c.Slug, viewer.ID,
				&contracts.AddCollectionItemRequest{EntityType: "artist", EntityID: other.ID})
			suite.NoError(err, "add item after reopening")
			suite.NoError(suite.collectionService.Subscribe(c.Slug, viewer.ID), "subscribe after reopening")
			_, err = suite.collectionService.Like(c.Slug, viewer.ID)
			suite.NoError(err, "like after reopening")
			_, err = suite.collectionService.SetFeatured(c.Slug, true, viewer.ID)
			suite.NoError(err, "feature after reopening")
		})
	}
}

// A viewer can still take back their own relation to a closed creator's
// collection. Unsubscribe and MarkVisited answer as they do for any slug and
// touch only the caller's row; Unlike removes the caller's like and answers the
// not-found error an unused slug gets, so the like and the slug are not
// published. Nobody else's like or subscription moves.
func (suite *CollectionServiceIntegrationTestSuite) TestClosedCreator_ViewerCanStillUndoTheirOwnRelation() {
	for i, state := range closedCreatorStates {
		suite.Run(state.name, func() {
			creator := suite.createTestUser(fmt.Sprintf("UndoCreator%d", i))
			viewer := suite.createTestUser(fmt.Sprintf("UndoViewer%d", i))
			bystander := suite.createTestUser(fmt.Sprintf("UndoBystander%d", i))
			c := suite.createPublicCollection(creator, fmt.Sprintf("Undo Picks %d", i))
			for _, u := range []*authm.User{viewer, bystander} {
				suite.Require().NoError(suite.collectionService.Subscribe(c.Slug, u.ID))
				_, err := suite.collectionService.Like(c.Slug, u.ID)
				suite.Require().NoError(err)
			}

			state.close(suite, creator.ID)

			suite.NoError(suite.collectionService.MarkVisited(c.Slug, viewer.ID))
			suite.NoError(suite.collectionService.Unsubscribe(c.Slug, viewer.ID))
			_, err := suite.collectionService.Unlike(c.Slug, viewer.ID)
			suite.requireCollectionNotFound(err, c.Slug, "Unlike on a closed creator's collection")
			_, err = suite.collectionService.Unlike("never-used-undo-slug", viewer.ID)
			suite.requireCollectionNotFound(err, "never-used-undo-slug", "Unlike on an unused slug")

			countRows := func(model interface{}, userID uint) int64 {
				var n int64
				suite.Require().NoError(suite.db.Model(model).
					Where("collection_id = ? AND user_id = ?", c.ID, userID).Count(&n).Error)
				return n
			}
			suite.Zero(countRows(&communitym.CollectionSubscriber{}, viewer.ID), "the viewer left")
			suite.Zero(countRows(&communitym.CollectionLike{}, viewer.ID), "the viewer's like is gone")
			suite.EqualValues(1, countRows(&communitym.CollectionSubscriber{}, bystander.ID), "a bystander's subscription stays")
			suite.EqualValues(1, countRows(&communitym.CollectionLike{}, bystander.ID), "a bystander's like stays")
		})
	}
}

// The admin remedies still reach a closed creator's collection: un-featuring
// closes its open run, an update is applied and reported as committed with
// nothing to show, and deletion removes it.
func (suite *CollectionServiceIntegrationTestSuite) TestClosedCreator_AdminRemediesStillApply() {
	for i, state := range closedCreatorStates {
		suite.Run(state.name, func() {
			creator := suite.createTestUser(fmt.Sprintf("RemedyCreator%d", i))
			admin := suite.createTestUser(fmt.Sprintf("RemedyAdmin%d", i))
			c := suite.createPublicCollection(creator, fmt.Sprintf("Remedy Picks %d", i))
			_, err := suite.collectionService.SetFeatured(c.Slug, true, admin.ID)
			suite.Require().NoError(err)

			state.close(suite, creator.ID)

			_, err = suite.collectionService.SetFeatured(c.Slug, false, admin.ID)
			suite.Require().NoError(err, "un-featuring a closed creator's collection")
			snap := suite.snapshotCollectionWrites(c.ID)
			suite.False(snap.Featured)
			suite.Zero(snap.OpenRuns, "the open run is closed")

			private := false
			detail, id, err := suite.collectionService.UpdateCollection(c.Slug, admin.ID, true,
				&contracts.UpdateCollectionRequest{IsPublic: &private})
			suite.Require().NoError(err, "an admin update is committed")
			suite.Nil(detail, "and has nothing to show")
			suite.Equal(c.ID, id)
			var stored communitym.Collection
			suite.Require().NoError(suite.db.First(&stored, c.ID).Error)
			suite.False(stored.IsPublic)

			_, err = suite.collectionService.DeleteCollection(c.Slug, admin.ID, true)
			suite.Require().NoError(err, "an admin removes a closed creator's collection")
			var remaining int64
			suite.Require().NoError(suite.db.Model(&communitym.Collection{}).Where("id = ?", c.ID).Count(&remaining).Error)
			suite.Zero(remaining)
		})
	}
}

// Fork counts count the forks the public is served: a closed cloner's fork
// leaves the source's count on the detail and the listing, and returns when
// the cloner reopens.
func (suite *CollectionServiceIntegrationTestSuite) TestClosedCloner_ForkLeavesForkCounts() {
	creator := suite.createTestUser("ForkCountCreator")
	live := suite.createTestUser("ForkCountLive")
	deactivated := suite.createTestUser("ForkCountDeactivated")
	deleted := suite.createTestUser("ForkCountDeleted")
	src := suite.createPublicCollection(creator, "Fork Count Source")
	for _, u := range []*authm.User{live, deactivated, deleted} {
		_, err := suite.collectionService.CloneCollection(src.Slug, u.ID)
		suite.Require().NoError(err)
	}

	assertForks := func(want int, msg string) {
		detail, err := suite.collectionService.GetBySlug(src.Slug, 0)
		suite.Require().NoError(err)
		suite.Equal(want, detail.ForksCount, "detail: %s", msg)
		rows, _, err := suite.collectionService.ListCollections(
			contracts.CollectionFilters{PublicOnly: true, CreatorID: creator.ID}, 20, 0)
		suite.Require().NoError(err)
		suite.Require().Len(rows, 1)
		suite.Equal(want, rows[0].ForksCount, "listing: %s", msg)
	}

	assertForks(3, "all cloners live")
	suite.closeAccounts(deactivated, deleted)
	assertForks(1, "two cloners closed")
	suite.reopenAccounts(deactivated, deleted)
	assertForks(3, "cloners reopened")
}
