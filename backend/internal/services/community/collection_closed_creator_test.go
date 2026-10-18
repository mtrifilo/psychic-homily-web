package community

import (
	stderrors "errors"
	"strconv"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// closeAccounts deactivates one account and soft-deletes the other, the two
// ways an account stops being live.
func (suite *CollectionServiceIntegrationTestSuite) closeAccounts(deactivated, deleted *authm.User) {
	testutil.DeactivateAccount(suite.T(), suite.db, deactivated.ID)
	testutil.SoftDeleteAccount(suite.T(), suite.db, deleted.ID)
}

// reopenAccounts reverses closeAccounts.
func (suite *CollectionServiceIntegrationTestSuite) reopenAccounts(deactivated, deleted *authm.User) {
	testutil.ReopenAccount(suite.T(), suite.db, deactivated.ID)
	testutil.ReopenAccount(suite.T(), suite.db, deleted.ID)
}

// requireCollectionNotFound asserts err is the not-found answer a never-used
// identifier gets, naming exactly the identifier the caller supplied.
func (suite *CollectionServiceIntegrationTestSuite) requireCollectionNotFound(err error, identifier, msg string) {
	suite.Require().Error(err, msg)
	var collErr *apperrors.CollectionError
	suite.Require().True(stderrors.As(err, &collErr), "%s: %v", msg, err)
	suite.Equal(apperrors.CodeCollectionNotFound, collErr.Code, msg)
	suite.Equal(apperrors.ErrCollectionNotFound(identifier).Message, collErr.Message, msg)
}

// slugs collects the slugs of a listing page.
func listedSlugs(rows []*contracts.CollectionListResponse) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Slug)
	}
	return out
}

// A closed creator's public collection answers every slug and id reader with
// the not-found answer a never-used identifier gets, for an anonymous and an
// authenticated viewer, and reopening the account serves it again. The id
// reader names the id, never the slug it resolved.
func (suite *CollectionServiceIntegrationTestSuite) TestClosedCreator_SlugAndIDReadersAnswerNotFoundUntilReopened() {
	deactivated := suite.createTestUser("SlugDeactivated")
	deleted := suite.createTestUser("SlugDeleted")
	stranger := suite.createTestUser("SlugStranger")
	artist := suite.createTestArtist("Slug Reader Artist")

	byCreator := map[uint]*contracts.CollectionDetailResponse{}
	for _, u := range []*authm.User{deactivated, deleted} {
		c := suite.createPublicCollection(u, "Closed Creator Picks "+*u.FirstName)
		suite.addArtistItemToCollection(c.ID, artist.ID, u.ID)
		byCreator[u.ID] = c
	}

	served := func(c *contracts.CollectionDetailResponse, viewerID uint) {
		detail, err := suite.collectionService.GetBySlug(c.Slug, viewerID)
		suite.Require().NoError(err)
		suite.Equal(c.ID, detail.ID)
		_, err = suite.collectionService.GetByID(c.ID, viewerID)
		suite.Require().NoError(err)
		_, err = suite.collectionService.GetStats(c.Slug, viewerID)
		suite.Require().NoError(err)
		_, err = suite.collectionService.GetCollectionGraph(c.Slug, viewerID, nil)
		suite.Require().NoError(err)
	}
	refused := func(c *contracts.CollectionDetailResponse, viewerID uint) {
		_, err := suite.collectionService.GetBySlug(c.Slug, viewerID)
		suite.requireCollectionNotFound(err, c.Slug, "GetBySlug")
		_, err = suite.collectionService.GetByID(c.ID, viewerID)
		suite.requireCollectionNotFound(err, strconv.FormatUint(uint64(c.ID), 10), "GetByID")
		_, err = suite.collectionService.GetStats(c.Slug, viewerID)
		suite.requireCollectionNotFound(err, c.Slug, "GetStats")
		_, err = suite.collectionService.GetCollectionGraph(c.Slug, viewerID, nil)
		suite.requireCollectionNotFound(err, c.Slug, "GetCollectionGraph")
	}

	for _, c := range byCreator {
		for _, viewerID := range []uint{0, stranger.ID} {
			served(c, viewerID)
		}
	}

	suite.closeAccounts(deactivated, deleted)
	for _, c := range byCreator {
		for _, viewerID := range []uint{0, stranger.ID} {
			refused(c, viewerID)
		}
	}

	suite.reopenAccounts(deactivated, deleted)
	for _, c := range byCreator {
		for _, viewerID := range []uint{0, stranger.ID} {
			served(c, viewerID)
		}
	}
}

// A closed creator's public collections leave the browse list (rows and
// total), the entity's collections list, a subscriber's library and the
// library's containing-entity lookup, and reopening the account lists them
// again.
func (suite *CollectionServiceIntegrationTestSuite) TestClosedCreator_LeavesBrowseEntityListAndLibraryUntilReopened() {
	live := suite.createTestUser("ListLive")
	deactivated := suite.createTestUser("ListDeactivated")
	deleted := suite.createTestUser("ListDeleted")
	subscriber := suite.createTestUser("ListSubscriber")
	artist := suite.createTestArtist("Listed Artist")

	slugByCreator := map[uint]string{}
	slugByID := map[uint]string{}
	for _, u := range []*authm.User{live, deactivated, deleted} {
		c := suite.createPublicCollection(u, "Listed Picks "+*u.FirstName)
		suite.addArtistItemToCollection(c.ID, artist.ID, u.ID)
		suite.Require().NoError(suite.collectionService.Subscribe(c.Slug, subscriber.ID))
		slugByCreator[u.ID] = c.Slug
		slugByID[c.ID] = c.Slug
	}

	assertListed := func(want []string, msg string) {
		for _, viewerID := range []uint{0, subscriber.ID} {
			rows, total, err := suite.collectionService.ListCollections(
				contracts.CollectionFilters{PublicOnly: true, ViewerID: viewerID}, 20, 0)
			suite.Require().NoError(err)
			suite.ElementsMatch(want, listedSlugs(rows), "browse rows, viewer %d: %s", viewerID, msg)
			suite.EqualValues(len(want), total, "browse total, viewer %d: %s", viewerID, msg)

			entityRows, err := suite.collectionService.GetEntityCollections("artist", artist.ID, viewerID, 10)
			suite.Require().NoError(err)
			suite.ElementsMatch(want, listedSlugs(entityRows), "entity collections, viewer %d: %s", viewerID, msg)
		}

		library, total, err := suite.collectionService.GetUserCollections(subscriber.ID, "", 20, 0)
		suite.Require().NoError(err)
		suite.ElementsMatch(want, listedSlugs(library), "library rows: %s", msg)
		suite.EqualValues(len(want), total, "library total: %s", msg)

		containing, err := suite.collectionService.GetUserCollectionsContainingEntity(subscriber.ID, "artist", artist.ID)
		suite.Require().NoError(err)
		containingSlugs := make([]string, 0, len(containing))
		for _, item := range containing {
			containingSlugs = append(containingSlugs, slugByID[item.CollectionID])
		}
		suite.ElementsMatch(want, containingSlugs, "containing-entity lookup mirrors the library: %s", msg)
	}

	all := []string{slugByCreator[live.ID], slugByCreator[deactivated.ID], slugByCreator[deleted.ID]}
	assertListed(all, "all creators live")

	suite.closeAccounts(deactivated, deleted)
	assertListed([]string{slugByCreator[live.ID]}, "two creators closed")

	suite.reopenAccounts(deactivated, deleted)
	assertListed(all, "creators reopened")
}

// A fork of a closed creator's collection answers its source as a deleted
// source answers, on the detail and on every listing that carries the fork,
// and reopening the source's creator restores the attribution.
func (suite *CollectionServiceIntegrationTestSuite) TestClosedCreator_ForkSourceAnswersLikeDeletedUntilReopened() {
	deactivated := suite.createTestUser("ForkDeactivated")
	deleted := suite.createTestUser("ForkDeleted")
	cloner := suite.createTestUser("ForkCloner")

	var forks []*contracts.CollectionDetailResponse
	for _, u := range []*authm.User{deactivated, deleted} {
		src := suite.createPublicCollection(u, "Fork Source "+*u.FirstName)
		fork, err := suite.collectionService.CloneCollection(src.Slug, cloner.ID)
		suite.Require().NoError(err)
		forks = append(forks, fork)
	}

	assertAttributed := func(attributed bool, msg string) {
		for _, fork := range forks {
			detail, err := suite.collectionService.GetBySlug(fork.Slug, 0)
			suite.Require().NoError(err)
			suite.Equal(attributed, detail.ForkedFrom != nil, "detail snapshot: %s", msg)
			suite.Equal(attributed, detail.ForkedFromCollectionID != nil, "detail id: %s", msg)
		}

		browse, _, err := suite.collectionService.ListCollections(
			contracts.CollectionFilters{PublicOnly: true, CreatorID: cloner.ID}, 20, 0)
		suite.Require().NoError(err)
		library, _, err := suite.collectionService.GetUserCollections(cloner.ID, "", 20, 0)
		suite.Require().NoError(err)
		for name, rows := range map[string][]*contracts.CollectionListResponse{"browse": browse, "library": library} {
			suite.Len(rows, len(forks), "%s rows: %s", name, msg)
			for _, row := range rows {
				suite.Equal(attributed, row.ForkedFromCollectionID != nil, "%s row id: %s", name, msg)
			}
		}
	}

	assertAttributed(true, "source creators live")

	suite.closeAccounts(deactivated, deleted)
	assertAttributed(false, "source creators closed")

	suite.reopenAccounts(deactivated, deleted)
	assertAttributed(true, "source creators reopened")
}

// Counts of people on a collection (subscribers, contributors, likes) count
// live accounts only, on the detail, the stats and the listing; counts of
// things (items, the per-type breakdown) still count every row. Reopening the
// accounts counts them again. CreateCollection subscribes the live creator, so
// the subscriber count is one more than the people added here.
func (suite *CollectionServiceIntegrationTestSuite) TestCollectionPeopleCounts_CountOnlyLiveAccounts() {
	creator := suite.createTestUser("CountCreator")
	live := suite.createTestUser("CountLive")
	deactivated := suite.createTestUser("CountDeactivated")
	deleted := suite.createTestUser("CountDeleted")
	c := suite.createPublicCollection(creator, "Counted Picks")

	for i, u := range []*authm.User{live, deactivated, deleted} {
		suite.Require().NoError(suite.collectionService.Subscribe(c.Slug, u.ID))
		_, err := suite.collectionService.Like(c.Slug, u.ID)
		suite.Require().NoError(err)
		artist := suite.createTestArtist("Counted Artist " + strconv.Itoa(i))
		suite.addArtistItemToCollection(c.ID, artist.ID, u.ID)
	}

	assertCounts := func(people int, msg string) {
		detail, err := suite.collectionService.GetBySlug(c.Slug, 0)
		suite.Require().NoError(err)
		suite.Equal(people+1, detail.SubscriberCount, "detail subscribers: %s", msg)
		suite.Equal(people, detail.ContributorCount, "detail contributors: %s", msg)
		suite.Equal(people, detail.LikeCount, "detail likes: %s", msg)
		suite.Equal(3, detail.ItemCount, "detail items: %s", msg)

		stats, err := suite.collectionService.GetStats(c.Slug, 0)
		suite.Require().NoError(err)
		suite.Equal(people+1, stats.SubscriberCount, "stats subscribers: %s", msg)
		suite.Equal(people, stats.ContributorCount, "stats contributors: %s", msg)
		suite.Equal(3, stats.ItemCount, "stats items: %s", msg)
		suite.Equal(3, stats.EntityTypeCounts["artist"], "stats per-type: %s", msg)

		rows, _, err := suite.collectionService.ListCollections(
			contracts.CollectionFilters{PublicOnly: true}, 20, 0)
		suite.Require().NoError(err)
		suite.Require().Len(rows, 1, msg)
		suite.Equal(people+1, rows[0].SubscriberCount, "row subscribers: %s", msg)
		suite.Equal(people, rows[0].ContributorCount, "row contributors: %s", msg)
		suite.Equal(people, rows[0].LikeCount, "row likes: %s", msg)
		suite.Equal(3, rows[0].ItemCount, "row items: %s", msg)
		suite.Equal(3, rows[0].EntityTypeCounts["artist"], "row per-type: %s", msg)

		// The like response a live liker gets counts what the detail counts.
		unliked, err := suite.collectionService.Unlike(c.Slug, live.ID)
		suite.Require().NoError(err)
		suite.Equal(people-1, unliked.LikeCount, "unlike response: %s", msg)
		liked, err := suite.collectionService.Like(c.Slug, live.ID)
		suite.Require().NoError(err)
		suite.Equal(people, liked.LikeCount, "like response: %s", msg)
	}

	assertCounts(3, "all accounts live")

	suite.closeAccounts(deactivated, deleted)
	assertCounts(1, "two accounts closed")

	suite.reopenAccounts(deactivated, deleted)
	assertCounts(3, "accounts reopened")
}

// A live creator whose profile is private keeps their public collections off
// the browse list and the entity's collections list for every viewer but
// themselves, while each collection is still served by its own slug; making
// the profile public lists them again.
func (suite *CollectionServiceIntegrationTestSuite) TestPrivateProfileCreator_LeavesBrowseAndEntityListButKeepsSlug() {
	creator := suite.createTestUser("PrivateProfileCreator")
	stranger := suite.createTestUser("PrivateProfileStranger")
	artist := suite.createTestArtist("Private Profile Artist")
	c := suite.createPublicCollection(creator, "Private Profile Picks")
	suite.addArtistItemToCollection(c.ID, artist.ID, creator.ID)

	setVisibility := func(visibility string) {
		suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", creator.ID).
			Update("profile_visibility", visibility).Error)
	}
	listedTo := func(viewerID uint) (browse, entity bool) {
		rows, total, err := suite.collectionService.ListCollections(
			contracts.CollectionFilters{PublicOnly: true, ViewerID: viewerID}, 20, 0)
		suite.Require().NoError(err)
		suite.EqualValues(len(rows), total, "browse total agrees with rows, viewer %d", viewerID)
		entityRows, err := suite.collectionService.GetEntityCollections("artist", artist.ID, viewerID, 10)
		suite.Require().NoError(err)
		return len(rows) == 1, len(entityRows) == 1
	}

	setVisibility("private")
	for _, viewerID := range []uint{0, stranger.ID} {
		browse, entity := listedTo(viewerID)
		suite.False(browse, "browse, viewer %d", viewerID)
		suite.False(entity, "entity collections, viewer %d", viewerID)
		detail, err := suite.collectionService.GetBySlug(c.Slug, viewerID)
		suite.Require().NoError(err, "the slug still serves, viewer %d", viewerID)
		suite.Equal(c.ID, detail.ID)
	}
	browse, entity := listedTo(creator.ID)
	suite.True(browse, "the creator still sees their own collection in browse")
	suite.True(entity, "the creator still sees their own collection on the entity")

	setVisibility("public")
	for _, viewerID := range []uint{0, stranger.ID} {
		browse, entity := listedTo(viewerID)
		suite.True(browse, "browse after the profile is public, viewer %d", viewerID)
		suite.True(entity, "entity collections after the profile is public, viewer %d", viewerID)
	}
}
