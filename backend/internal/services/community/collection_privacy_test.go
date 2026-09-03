package community

import (
	stderrors "errors"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// Collection privacy: the surfaces a stale subscription or a public sibling
// reaches
// =============================================================================
//
// Each test here covers a path where a private collection is reachable WITHOUT
// naming it: through a subscription row that outlived the flip, through a
// public fork that points at it, or through a count rendered beside a filtered
// listing. The detail route is not the only door, and none of these is behind
// it.

// A SUBSCRIPTION IS NOT A GRANT. Subscribing is allowed while a collection is
// public and the row survives the creator turning it private, so the library
// listing has to re-check the rule on every read.
func (suite *CollectionServiceIntegrationTestSuite) TestGetUserCollections_HidesAPrivateCollectionASubscriberStillWatches() {
	creator := suite.createTestUser("libcreator")
	stranger := suite.createTestUser("libstranger")

	open := suite.createPublicCollection(creator, "Library Open")
	shut := suite.createPublicCollection(creator, "Library Shut")

	suite.Require().NoError(suite.collectionService.Subscribe(open.Slug, stranger.ID))
	suite.Require().NoError(suite.collectionService.Subscribe(shut.Slug, stranger.ID))

	// The control: both are visible while both are public.
	before, beforeTotal, err := suite.collectionService.GetUserCollections(stranger.ID, "", 50, 0)
	suite.Require().NoError(err)
	suite.Require().Len(before, 2)
	suite.EqualValues(len(before), beforeTotal)

	suite.Require().NoError(suite.db.Table("collections").
		Where("id = ?", shut.ID).Update("is_public", false).Error)

	after, afterTotal, err := suite.collectionService.GetUserCollections(stranger.ID, "", 50, 0)
	suite.Require().NoError(err)
	suite.Require().Len(after, 1, "a collection turned private leaves the subscriber's library")
	suite.Equal(open.ID, after[0].ID)
	suite.EqualValues(len(after), afterTotal,
		"the total must count the same rows the page contains")

	// The creator keeps their own private collection, which is what makes the
	// assertion above about visibility rather than about the query breaking.
	own, ownTotal, err := suite.collectionService.GetUserCollections(creator.ID, "", 50, 0)
	suite.Require().NoError(err)
	suite.Len(own, 2)
	suite.EqualValues(len(own), ownTotal)
}

// The membership popover reaches the same subscribed collections and must apply
// the same rule, or it confirms what a private collection holds.
func (suite *CollectionServiceIntegrationTestSuite) TestGetUserCollectionsContainingEntity_HidesAPrivateSubscribedCollection() {
	creator := suite.createTestUser("containscreator")
	stranger := suite.createTestUser("containsstranger")
	artist := suite.createTestArtist("Contains Artist")

	shut := suite.createPublicCollection(creator, "Contains Shut")
	suite.Require().NoError(suite.collectionService.Subscribe(shut.Slug, stranger.ID))
	_, err := suite.collectionService.AddItem(shut.Slug, creator.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
	})
	suite.Require().NoError(err)

	before, err := suite.collectionService.GetUserCollectionsContainingEntity(stranger.ID, "artist", artist.ID)
	suite.Require().NoError(err)
	suite.Require().Len(before, 1, "the control: a public subscribed collection reports its membership")

	suite.Require().NoError(suite.db.Table("collections").
		Where("id = ?", shut.ID).Update("is_public", false).Error)

	after, err := suite.collectionService.GetUserCollectionsContainingEntity(stranger.ID, "artist", artist.ID)
	suite.Require().NoError(err)
	suite.Empty(after, "a private collection must not confirm what it contains to a subscriber")

	// The creator still sees their own, so the popover keeps working for the one
	// person who can curate it.
	own, err := suite.collectionService.GetUserCollectionsContainingEntity(creator.ID, "artist", artist.ID)
	suite.Require().NoError(err)
	suite.Len(own, 1)
}

// THE COUNTS ARE THE ACTIVITY. A stats route that answered for a private
// collection would report how many items it holds and how many people watch it,
// and would sort real slugs from unused ones.
func (suite *CollectionServiceIntegrationTestSuite) TestGetStats_RefusesAPrivateCollectionLikeAMissingOne() {
	creator := suite.createTestUser("statscreator")
	stranger := suite.createTestUser("statsstranger")
	shut := suite.createBasicCollection(creator, "Stats Shut")

	// The two errors differ only in the slug each echoes back, which is the
	// caller's own input. The CODE is what the handler maps to a status, so that
	// is what has to match.
	_, err := suite.collectionService.GetStats(shut.Slug, stranger.ID)
	suite.Require().Error(err)
	_, missingErr := suite.collectionService.GetStats("a-slug-that-never-existed", stranger.ID)
	suite.Require().Error(missingErr)
	var gated, missing *apperrors.CollectionError
	suite.Require().True(stderrors.As(err, &gated))
	suite.Require().True(stderrors.As(missingErr, &missing))
	suite.Equal(missing.Code, gated.Code,
		"a private collection and a slug nobody has used must answer alike")

	_, anonErr := suite.collectionService.GetStats(shut.Slug, 0)
	suite.Require().Error(anonErr)

	// The creator gets the real answer, which is the control.
	stats, err := suite.collectionService.GetStats(shut.Slug, creator.ID)
	suite.Require().NoError(err)
	suite.NotNil(stats)
}

// A PUBLIC FORK MUST NOT REPUBLISH ITS PRIVATE SOURCE. The clone is a
// collection of its own and CloneCollection creates it public whatever the
// source was, so the attribution snapshot is a read of the source.
func (suite *CollectionServiceIntegrationTestSuite) TestGetBySlug_HidesAPrivateForkSource() {
	creator := suite.createTestUser("forkcreator")
	stranger := suite.createTestUser("forkstranger")

	source := suite.createPublicCollection(creator, "Fork Source")
	clone, err := suite.collectionService.CloneCollection(source.Slug, stranger.ID)
	suite.Require().NoError(err)

	before, err := suite.collectionService.GetBySlug(clone.Slug, stranger.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(before.ForkedFrom, "the control: a public source is attributed")

	suite.Require().NoError(suite.db.Table("collections").
		Where("id = ?", source.ID).Update("is_public", false).Error)

	after, err := suite.collectionService.GetBySlug(clone.Slug, stranger.ID)
	suite.Require().NoError(err)
	suite.Nil(after.ForkedFrom,
		"a private source must answer like a deleted one: no title, no slug, no curator")

	// The source's own creator keeps the attribution.
	owned, err := suite.collectionService.GetBySlug(clone.Slug, creator.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(owned.ForkedFrom)
	suite.Equal(source.Slug, owned.ForkedFrom.Slug)
}

// A COLLECTION THE CALLER CANNOT READ IS NOT ONE THEY MAY TAG, whatever
// `collaborative` says. The slug-addressed tag write returns the collection's
// tag list, so it is a read as well as a write.
func (suite *CollectionServiceIntegrationTestSuite) TestAddTagToCollection_RefusesAPrivateCollaborativeCollection() {
	creator := suite.createTestUser("tagcreator")
	stranger := suite.createTestUser("tagstranger")

	open, err := suite.collectionService.CreateCollection(creator.ID, &contracts.CreateCollectionRequest{
		Title:         "Tag Collaborative",
		IsPublic:      true,
		Collaborative: true,
	})
	suite.Require().NoError(err)

	// The control: a stranger may tag a PUBLIC collaborative collection.
	tagged, err := suite.collectionService.AddTagToCollection(open.Slug, stranger.ID,
		&contracts.AddCollectionTagRequest{TagName: "public-collab-tag"})
	suite.Require().NoError(err)
	suite.Require().Len(tagged.Tags, 1)
	tagID := tagged.Tags[0].TagID

	suite.Require().NoError(suite.db.Table("collections").
		Where("id = ?", open.ID).Update("is_public", false).Error)

	_, err = suite.collectionService.AddTagToCollection(open.Slug, stranger.ID,
		&contracts.AddCollectionTagRequest{TagName: "private-collab-tag"})
	suite.Require().Error(err, "a stranger may not tag a private collaborative collection")
	var addErr *apperrors.CollectionError
	suite.Require().True(stderrors.As(err, &addErr))
	// NOT FOUND, on the terms GetBySlug states: an invisible collection and one
	// that does not exist answer alike.
	suite.Equal(apperrors.CodeCollectionNotFound, addErr.Code)

	// RemoveTagFromCollection takes (slug, tagID, userID), the opposite argument
	// order to AddTagToCollection. Passing a tag that IS on the collection is
	// what makes this about the gate rather than about a missing tag.
	err = suite.collectionService.RemoveTagFromCollection(open.Slug, tagID, stranger.ID)
	suite.Require().Error(err, "nor remove one")
	var removeErr *apperrors.CollectionError
	suite.Require().True(stderrors.As(err, &removeErr))
	suite.Equal(apperrors.CodeCollectionNotFound, removeErr.Code)

	// The creator still can, so the gate did not break the feature.
	_, err = suite.collectionService.AddTagToCollection(open.Slug, creator.ID, &contracts.AddCollectionTagRequest{
		TagName: "creator-tag",
	})
	suite.Require().NoError(err)
}

// THE ITEM WRITES, and they are the loudest of the family: a bulk add reports a
// per-row duplicate for every entity already present, so an ungated one
// enumerates a private collection's contents 200 rows at a time.
func (suite *CollectionServiceIntegrationTestSuite) TestAddItem_RefusesAPrivateCollaborativeCollection() {
	creator := suite.createTestUser("additemcreator")
	stranger := suite.createTestUser("additemstranger")
	artist := suite.createTestArtist("Add Item Artist")
	other := suite.createTestArtist("Add Item Other Artist")

	open, err := suite.collectionService.CreateCollection(creator.ID, &contracts.CreateCollectionRequest{
		Title:         "Add Item Collaborative",
		IsPublic:      true,
		Collaborative: true,
	})
	suite.Require().NoError(err)

	// The control: a stranger may add to a PUBLIC collaborative collection.
	_, err = suite.collectionService.AddItem(open.Slug, stranger.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
	})
	suite.Require().NoError(err)

	suite.Require().NoError(suite.db.Table("collections").
		Where("id = ?", open.ID).Update("is_public", false).Error)

	_, err = suite.collectionService.AddItem(open.Slug, stranger.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   other.ID,
	})
	suite.Require().Error(err)
	var addErr *apperrors.CollectionError
	suite.Require().True(stderrors.As(err, &addErr))
	suite.Equal(apperrors.CodeCollectionNotFound, addErr.Code,
		"a private collaborative collection answers an outsider like a missing one")

	// The creator still can, so the gate did not break the feature.
	_, err = suite.collectionService.AddItem(open.Slug, creator.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   other.ID,
	})
	suite.Require().NoError(err)
}

func (suite *CollectionServiceIntegrationTestSuite) TestBulkAddItems_RefusesAPrivateCollaborativeCollection() {
	creator := suite.createTestUser("bulkcreator")
	stranger := suite.createTestUser("bulkstranger")
	artist := suite.createTestArtist("Bulk Item Artist")

	open, err := suite.collectionService.CreateCollection(creator.ID, &contracts.CreateCollectionRequest{
		Title:         "Bulk Add Collaborative",
		IsPublic:      true,
		Collaborative: true,
	})
	suite.Require().NoError(err)

	_, err = suite.collectionService.AddItem(open.Slug, creator.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
	})
	suite.Require().NoError(err)

	req := &contracts.BulkAddCollectionItemsRequest{
		Items: []contracts.AddCollectionItemRequest{{EntityType: "artist", EntityID: artist.ID}},
	}

	// The control: while it is public the duplicate is reported, which is exactly
	// the row-by-row disclosure the gate has to stop once it is private.
	before, err := suite.collectionService.BulkAddItems(open.Slug, stranger.ID, req)
	suite.Require().NoError(err)
	suite.Require().Len(before.Errors, 1, "the control: a duplicate is reported on a public collection")

	suite.Require().NoError(suite.db.Table("collections").
		Where("id = ?", open.ID).Update("is_public", false).Error)

	after, err := suite.collectionService.BulkAddItems(open.Slug, stranger.ID, req)
	suite.Require().Error(err)
	suite.Nil(after, "no per-row report may be returned for a collection the caller cannot see")
	var bulkErr *apperrors.CollectionError
	suite.Require().True(stderrors.As(err, &bulkErr))
	suite.Equal(apperrors.CodeCollectionNotFound, bulkErr.Code)
}

// CLONING IS A READ. It copies the source's title, description and every item
// into a collection the caller owns, so a source the caller cannot see answers
// like one that does not exist rather than like one they are forbidden.
func (suite *CollectionServiceIntegrationTestSuite) TestCloneCollection_RefusesAPrivateSourceLikeAMissingOne() {
	creator := suite.createTestUser("clonecreator")
	stranger := suite.createTestUser("clonestranger")
	shut := suite.createBasicCollection(creator, "Clone Shut")

	_, err := suite.collectionService.CloneCollection(shut.Slug, stranger.ID)
	suite.Require().Error(err)
	_, missingErr := suite.collectionService.CloneCollection("a-slug-that-never-existed", stranger.ID)
	suite.Require().Error(missingErr)

	var gated, missing *apperrors.CollectionError
	suite.Require().True(stderrors.As(err, &gated))
	suite.Require().True(stderrors.As(missingErr, &missing))
	suite.Equal(missing.Code, gated.Code,
		"a private source and a slug nobody has used must answer alike")

	// The creator can still fork their own, which is the control.
	clone, err := suite.collectionService.CloneCollection(shut.Slug, creator.ID)
	suite.Require().NoError(err)
	suite.NotNil(clone)
}

// THE OWNER WRITES ANSWER A NON-CREATOR AS A MISSING COLLECTION, and the item
// writes decide the collection BEFORE they load the item: item ids are a global
// sequence, so an item lookup that ran first would sort ids that belong to this
// collection from ids that do not.
func (suite *CollectionServiceIntegrationTestSuite) TestOwnerWritesRefuseAPrivateCollectionLikeAMissingOne() {
	creator := suite.createTestUser("ownerwritecreator")
	stranger := suite.createTestUser("ownerwritestranger")
	artist := suite.createTestArtist("Owner Write Artist")

	shut := suite.createBasicCollection(creator, "Owner Write Shut")
	item, err := suite.collectionService.AddItem(shut.Slug, creator.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
	})
	suite.Require().NoError(err)

	const missingSlug = "a-slug-that-never-existed"
	notes := "a stranger should not be able to write this"

	codeOf := func(err error) string {
		suite.Require().Error(err)
		var collErr *apperrors.CollectionError
		suite.Require().True(stderrors.As(err, &collErr))
		return collErr.Code
	}

	// UpdateCollection and DeleteCollection admit an admin (the moderation
	// remedies), so the stranger is the caller here and the admin arm is asserted
	// in the route matrix.
	_, updateErr := suite.collectionService.UpdateCollection(shut.Slug, stranger.ID, false,
		&contracts.UpdateCollectionRequest{Title: &notes})
	_, updateMissingErr := suite.collectionService.UpdateCollection(missingSlug, stranger.ID, false,
		&contracts.UpdateCollectionRequest{Title: &notes})
	suite.Equal(codeOf(updateMissingErr), codeOf(updateErr), "PUT /collections/{slug}")

	suite.Equal(
		codeOf(suite.collectionService.DeleteCollection(missingSlug, stranger.ID, false)),
		codeOf(suite.collectionService.DeleteCollection(shut.Slug, stranger.ID, false)),
		"DELETE /collections/{slug}")

	// The item writes, with an item id that IS in this collection. A gate placed
	// after the item lookup would answer item-not-found here and forbidden for an
	// id from another collection, and that pair is membership.
	_, itemErr := suite.collectionService.UpdateItem(shut.Slug, item.ID, stranger.ID, false,
		&contracts.UpdateCollectionItemRequest{Notes: &notes})
	_, itemMissingErr := suite.collectionService.UpdateItem(missingSlug, item.ID, stranger.ID, false,
		&contracts.UpdateCollectionItemRequest{Notes: &notes})
	suite.Equal(codeOf(itemMissingErr), codeOf(itemErr), "PATCH /collections/{slug}/items/{item_id}")

	suite.Equal(
		codeOf(suite.collectionService.RemoveItem(missingSlug, item.ID, stranger.ID, false)),
		codeOf(suite.collectionService.RemoveItem(shut.Slug, item.ID, stranger.ID, false)),
		"DELETE /collections/{slug}/items/{item_id}")

	reorder := &contracts.ReorderCollectionItemsRequest{
		Items: []contracts.ReorderItem{{ItemID: item.ID, Position: 1}},
	}
	suite.Equal(
		codeOf(suite.collectionService.ReorderItems(missingSlug, stranger.ID, reorder)),
		codeOf(suite.collectionService.ReorderItems(shut.Slug, stranger.ID, reorder)),
		"PUT /collections/{slug}/items/reorder")

	// AN ADMIN IS REFUSED THE ITEM WRITES, and admitted to the two moderation
	// remedies. Both halves are the assertion.
	_, adminItemErr := suite.collectionService.UpdateItem(shut.Slug, item.ID, stranger.ID, true,
		&contracts.UpdateCollectionItemRequest{Notes: &notes})
	suite.Equal(apperrors.CodeCollectionNotFound, codeOf(adminItemErr),
		"an admin is refused a private collection's item writes")

	// The creator still holds all of them, which is the control.
	_, err = suite.collectionService.UpdateItem(shut.Slug, item.ID, creator.ID, false,
		&contracts.UpdateCollectionItemRequest{Notes: &notes})
	suite.Require().NoError(err)
	suite.Require().NoError(suite.collectionService.ReorderItems(shut.Slug, creator.ID, reorder))
	suite.Require().NoError(suite.collectionService.RemoveItem(shut.Slug, item.ID, creator.ID, false))
}

// UNSUBSCRIBING NEVER REPORTS WHETHER THE SLUG EXISTS. Quitting is always
// allowed, so a slug nobody has used and a private collection's guessable name
// answer the same success.
func (suite *CollectionServiceIntegrationTestSuite) TestUnsubscribe_IsNotASlugExistenceOracle() {
	creator := suite.createTestUser("unsubcreator")
	stranger := suite.createTestUser("unsubstranger")

	shut := suite.createPublicCollection(creator, "Unsub Shut")
	suite.Require().NoError(suite.collectionService.Subscribe(shut.Slug, stranger.ID))
	suite.Require().NoError(suite.db.Table("collections").
		Where("id = ?", shut.ID).Update("is_public", false).Error)

	suite.Require().NoError(suite.collectionService.Unsubscribe(shut.Slug, stranger.ID),
		"a subscriber may always quit")
	suite.Require().NoError(suite.collectionService.Unsubscribe(shut.Slug, stranger.ID),
		"and quitting twice answers the same")
	suite.Require().NoError(suite.collectionService.Unsubscribe("a-slug-that-never-existed", stranger.ID),
		"an unused slug answers the same as a real one")

	// The row is gone, which is what makes the successes above about the delete
	// rather than about a no-op.
	var remaining int64
	suite.Require().NoError(suite.db.Table("collection_subscribers").
		Where("collection_id = ? AND user_id = ?", shut.ID, stranger.ID).
		Count(&remaining).Error)
	suite.Zero(remaining)
}
