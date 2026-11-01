package catalog

// The catalog listings carry a collection only on the public-tier listing rule
// (shared.PublicListedCollectionSQL): public, with a live creator whose profile
// is public. A closed creator (deactivated or soft-deleted) and a private-profile
// creator take their public collections off the featured card and archive, the
// scene rail, tag pages and the tag intersection, and the counts beside them;
// reopening or publishing the profile lists them again. The popular charts
// count live followers only.

import (
	"fmt"
	"time"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/testutil"
)

func (suite *ChartsServiceIntegrationTestSuite) TestFeaturedReads_ListOnlyLiveCreatorsWithPublicProfiles() {
	live := suite.createUser("feat-live@test.com")
	deactivated := suite.createUser("feat-deactivated@test.com")
	deleted := suite.createUser("feat-deleted@test.com")
	private := suite.createUser("feat-private@test.com")
	liveSub := suite.createUser("feat-live-sub@test.com")
	closedSub := suite.createUser("feat-closed-sub@test.com")

	now := time.Now().UTC()
	control := suite.createCollection(live.ID, "Live Creator Pick", "live-creator-pick")
	suite.insertRun(control.ID, now.Add(-96*time.Hour), nil, false)
	suite.subscribe(control.ID, liveSub.ID)
	suite.subscribe(control.ID, closedSub.ID)
	// Each hidden creator's run is NEWER than the control's, so only the
	// listing rule can keep it off the live pick. The deactivated creator's is
	// the newest.
	var deactivatedPick *communitym.Collection
	for i, u := range []*authm.User{deactivated, deleted, private} {
		c := suite.createCollection(u.ID, fmt.Sprintf("Hidden Creator Pick %d", i), fmt.Sprintf("hidden-creator-pick-%d", i))
		suite.insertRun(c.ID, now.Add(-time.Duration(i+1)*time.Hour), nil, false)
		if u.ID == deactivated.ID {
			deactivatedPick = c
		}
	}

	testutil.DeactivateAccount(suite.T(), suite.db, deactivated.ID)
	testutil.SoftDeleteAccount(suite.T(), suite.db, deleted.ID)
	testutil.DeactivateAccount(suite.T(), suite.db, closedSub.ID)
	testutil.SetProfileVisibility(suite.T(), suite.db, private.ID, "private")

	pick, err := suite.chartsService.GetFeaturedCollection()
	suite.Require().NoError(err)
	suite.Require().NotNil(pick)
	suite.Equal(control.ID, pick.CollectionID, "only the live, public-profile creator's pick is listed")
	suite.Equal(1, pick.SubscriberCount, "a closed subscriber is not counted")

	runs, total, err := suite.chartsService.GetFeaturedCollectionHistory(20, 0)
	suite.Require().NoError(err)
	suite.Equal(1, total, "the archive total counts the listed runs only")
	suite.Require().Len(runs, 1)
	suite.Equal(control.ID, runs[0].CollectionID)

	// Reopening the deactivated creator lists their newer pick again.
	testutil.ReopenAccount(suite.T(), suite.db, deactivated.ID)
	pick, err = suite.chartsService.GetFeaturedCollection()
	suite.Require().NoError(err)
	suite.Require().NotNil(pick)
	suite.Equal(deactivatedPick.ID, pick.CollectionID)
	_, total, err = suite.chartsService.GetFeaturedCollectionHistory(20, 0)
	suite.Require().NoError(err)
	suite.Equal(2, total)
}

func (suite *ChartsServiceIntegrationTestSuite) TestPopularCharts_CountLiveFollowersOnly() {
	artist := suite.createArtist("Followed By Mixed Accounts")
	venue := suite.createVenue("Room Followed By Mixed Accounts", "Phoenix", "AZ")

	var followers []*authm.User
	for i := 0; i < 4; i++ {
		u := suite.createUser(fmt.Sprintf("popular-follower-%d@test.com", i))
		suite.createBookmark(u.ID, engagementm.BookmarkEntityArtist, artist.ID, engagementm.BookmarkActionFollow)
		suite.createBookmark(u.ID, engagementm.BookmarkEntityVenue, venue.ID, engagementm.BookmarkActionFollow)
		followers = append(followers, u)
	}
	testutil.DeactivateAccount(suite.T(), suite.db, followers[2].ID)
	testutil.SoftDeleteAccount(suite.T(), suite.db, followers[3].ID)

	artists, err := suite.chartsService.GetPopularArtists(10)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 1)
	suite.Equal(2, artists[0].FollowCount)
	suite.Equal(4, artists[0].Score, "score = follow_count * 2 + upcoming shows")

	venues, err := suite.chartsService.GetActiveVenues(10)
	suite.Require().NoError(err)
	suite.Require().Len(venues, 1)
	suite.Equal(2, venues[0].FollowCount)

	testutil.ReopenAccount(suite.T(), suite.db, followers[2].ID)
	artists, err = suite.chartsService.GetPopularArtists(10)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 1)
	suite.Equal(3, artists[0].FollowCount, "a reopened follower counts again")
}

func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCollections_ListsOnlyLiveCreatorsWithPublicProfiles() {
	suite.seedPhoenixRooms()
	live := suite.createUser()
	deactivated := suite.createUser()
	deleted := suite.createUser()
	private := suite.createUser()
	closedHelper := suite.createUser()

	local := make([]uint, 0, 6)
	for i := 0; i < 6; i++ {
		local = append(local, suite.createArtist(fmt.Sprintf("Phoenix Listing Band %d", i)).ID)
	}

	// 6 of 6 local: every collection qualifies under both arms, so only the
	// listing rule can keep one off the rail.
	control := suite.createCollection("Live Creator Phoenix", live.ID, true)
	suite.addItems(control, live.ID, communitym.CollectionEntityArtist, local[:3]...)
	suite.addItems(control, closedHelper.ID, communitym.CollectionEntityArtist, local[3:]...)
	byCreator := map[uint]string{}
	for i, u := range []*authm.User{deactivated, deleted, private} {
		title := fmt.Sprintf("Hidden Creator Phoenix %d", i)
		c := suite.createCollection(title, u.ID, true)
		suite.addItems(c, u.ID, communitym.CollectionEntityArtist, local...)
		byCreator[u.ID] = title
	}

	testutil.DeactivateAccount(suite.T(), suite.db, deactivated.ID)
	testutil.SoftDeleteAccount(suite.T(), suite.db, deleted.ID)
	testutil.DeactivateAccount(suite.T(), suite.db, closedHelper.ID)
	testutil.SetProfileVisibility(suite.T(), suite.db, private.ID, "private")

	got, err := suite.sceneService.GetSceneCollections("Phoenix", "AZ", 10)
	suite.Require().NoError(err)
	suite.Equal([]string{"Live Creator Phoenix"}, collectionTitles(got))
	suite.Require().Len(got, 1)
	suite.Equal(1, got[0].ContributorCount, "a closed contributor is not counted")
	suite.Equal(6, got[0].ItemCount, "every item still counts")

	testutil.ReopenAccount(suite.T(), suite.db, deactivated.ID)
	testutil.SetProfileVisibility(suite.T(), suite.db, private.ID, "public")
	got, err = suite.sceneService.GetSceneCollections("Phoenix", "AZ", 10)
	suite.Require().NoError(err)
	suite.ElementsMatch([]string{"Live Creator Phoenix", byCreator[deactivated.ID], byCreator[private.ID]},
		collectionTitles(got))
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagEntities_Collections_ListOnlyLiveCreatorsWithPublicProfiles() {
	live := suite.createTestUserWithTier("listing-live", "contributor")
	deactivated := suite.createTestUserWithTier("listing-deactivated", "contributor")
	deleted := suite.createTestUserWithTier("listing-deleted", "contributor")
	private := suite.createTestUserWithTier("listing-private", "contributor")
	tag := suite.createTag("listing-rule", "other")

	control := suite.createCollection(live.ID, "Listed Picks", true)
	_, err := suite.tagService.AddTagToEntity(tag.ID, "", catalogm.TagEntityCollection, control.ID, live.ID, "")
	suite.Require().NoError(err)
	ids := map[uint]uint{}
	for _, u := range []*authm.User{deactivated, deleted, private} {
		c := suite.createCollection(u.ID, "Hidden Picks "+fmt.Sprint(u.ID), true)
		_, err := suite.tagService.AddTagToEntity(tag.ID, "", catalogm.TagEntityCollection, c.ID, u.ID, "")
		suite.Require().NoError(err)
		ids[u.ID] = c.ID
	}

	testutil.DeactivateAccount(suite.T(), suite.db, deactivated.ID)
	testutil.SoftDeleteAccount(suite.T(), suite.db, deleted.ID)
	testutil.SetProfileVisibility(suite.T(), suite.db, private.ID, "private")

	assertListed := func(want []uint, msg string) {
		rows, total, err := suite.tagService.GetTagEntities(tag.ID, catalogm.TagEntityCollection, 50, 0)
		suite.Require().NoError(err)
		got := make([]uint, 0, len(rows))
		for _, r := range rows {
			got = append(got, r.EntityID)
		}
		suite.ElementsMatch(want, got, "rows: %s", msg)
		suite.EqualValues(len(want), total, "total: %s", msg)

		// The counts beside the listing count the same rows.
		detail, err := suite.tagService.GetTagDetail(tag.ID)
		suite.Require().NoError(err)
		suite.Require().NotNil(detail)
		suite.EqualValues(len(want), detail.UsageBreakdown[catalogm.TagEntityCollection], "breakdown: %s", msg)
		counts, err := shared.ListedTagUsageCounts(suite.db, []uint{tag.ID})
		suite.Require().NoError(err)
		suite.Equal(len(want), counts[tag.ID], "usage count: %s", msg)

		// The collection facet on the tag browse counts the same rows.
		facet, _, err := suite.tagService.ListTags("", "", nil, "", 50, 0, catalogm.TagEntityCollection, nil)
		suite.Require().NoError(err)
		facetCount := -1
		for _, t := range facet {
			if t.ID == tag.ID {
				facetCount = t.UsageCount
			}
		}
		suite.Equal(len(want), facetCount, "collection facet count: %s", msg)

		// Each tagger is credited only with the rows the page lists, so a
		// private-profile creator's own tagging is not counted against them.
		credited := map[uint]int64{}
		for _, c := range detail.TopContributors {
			credited[c.User.ID] = c.Count
		}
		suite.Equal(int64(1), credited[live.ID], "top contributors, live tagger: %s", msg)
		suite.Equal(int64(len(want)-1), credited[deleted.ID]+credited[private.ID], "top contributors, hidden taggers: %s", msg)
	}

	assertListed([]uint{control.ID}, "closed and private-profile creators hidden")

	testutil.ReopenAccount(suite.T(), suite.db, deleted.ID)
	testutil.SetProfileVisibility(suite.T(), suite.db, private.ID, "public")
	assertListed([]uint{control.ID, ids[deleted.ID], ids[private.ID]}, "reopened and profile published")
}

func (s *TagIntersectionIntegrationTestSuite) TestIntersection_CollectionListsOnlyLiveCreatorsWithPublicProfiles() {
	seedUser := func(name string) *authm.User {
		email := fmt.Sprintf("%s-%d@test.com", name, time.Now().UnixNano())
		u := &authm.User{Email: &email, IsActive: true, EmailVerified: true}
		s.Require().NoError(s.db.Create(u).Error)
		return u
	}
	seedCollectionBy := func(title string, creatorID uint) uint {
		slug := fmt.Sprintf("%s-%d", title, time.Now().UnixNano())
		c := &communitym.Collection{Title: title, Slug: slug, CreatorID: creatorID, IsPublic: true, DisplayMode: "unranked"}
		s.Require().NoError(s.db.Create(c).Error)
		return c.ID
	}

	live := seedUser("intersect-live")
	deactivated := seedUser("intersect-deactivated")
	deleted := seedUser("intersect-deleted")
	private := seedUser("intersect-private")
	ids := []uint{
		seedCollectionBy("LiveCreatorCol", live.ID),
		seedCollectionBy("DeactivatedCreatorCol", deactivated.ID),
		seedCollectionBy("DeletedCreatorCol", deleted.ID),
		seedCollectionBy("PrivateProfileCol", private.ID),
	}
	for _, id := range ids {
		s.tag("collection", id, "shoegaze")
		s.tag("collection", id, "electronic")
	}
	testutil.DeactivateAccount(s.T(), s.db, deactivated.ID)
	testutil.SoftDeleteAccount(s.T(), s.db, deleted.ID)
	testutil.SetProfileVisibility(s.T(), s.db, private.ID, "private")

	collectionGroup := func() (int64, []string) {
		resp, err := s.tagService.IntersectEntitiesByTags([]string{"shoegaze", "electronic"}, false, 4)
		s.Require().NoError(err)
		for _, g := range resp.Groups {
			if g.EntityType == "collection" {
				names := make([]string, 0, len(g.Preview))
				for _, p := range g.Preview {
					names = append(names, p.Name)
				}
				return g.Count, names
			}
		}
		s.FailNow("no collection group")
		return 0, nil
	}

	count, names := collectionGroup()
	s.EqualValues(1, count)
	s.Equal([]string{"LiveCreatorCol"}, names)

	testutil.ReopenAccount(s.T(), s.db, deactivated.ID)
	testutil.ReopenAccount(s.T(), s.db, deleted.ID)
	count, names = collectionGroup()
	s.EqualValues(3, count)
	s.ElementsMatch([]string{"LiveCreatorCol", "DeactivatedCreatorCol", "DeletedCreatorCol"}, names)
}
