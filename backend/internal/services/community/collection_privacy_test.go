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
