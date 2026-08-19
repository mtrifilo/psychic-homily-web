package catalog

import (
	"fmt"
	"testing"
	"time"

	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

// Scene-scoped public collections (PSY-1847) — run as part of the
// SceneServiceIntegrationTestSuite (real Postgres, all migrations).
//
// The rule under test: a public collection qualifies for a scene when at least
// half its members are scene-local OR at least five of them are, ranked by
// scene-local member count. Scene-local means an artist BASED in the scene, a
// venue in the scene, or a show at a scene venue; releases, labels and
// festivals count toward the total but can never be local.

// =============================================================================
// FIXTURE HELPERS
// =============================================================================

// createCollection seeds a collection owned by creatorID.
//
// is_public is written with a follow-up Update rather than on Create because
// `false` is Go's zero value and GORM drops it, letting the column DEFAULT
// (true) win — the exact way a "private collection stays hidden" test would
// silently seed a PUBLIC row and then pass for the wrong reason. The assertion
// below re-reads the row so the fixture cannot lie about what it seeded.
func (suite *SceneServiceIntegrationTestSuite) createCollection(
	title string, creatorID uint, isPublic bool,
) *communitym.Collection {
	c := &communitym.Collection{
		Title:       title,
		Slug:        fmt.Sprintf("c-%d", time.Now().UnixNano()),
		CreatorID:   creatorID,
		IsPublic:    true,
		DisplayMode: communitym.CollectionDisplayModeUnranked,
	}
	suite.Require().NoError(suite.db.Create(c).Error)
	if !isPublic {
		suite.Require().NoError(suite.db.Model(c).Update("is_public", false).Error)
	}

	var persisted communitym.Collection
	suite.Require().NoError(suite.db.First(&persisted, c.ID).Error)
	suite.Require().Equal(isPublic, persisted.IsPublic,
		"fixture must persist the visibility it claims, or the privacy assertion is vacuous")
	return c
}

// addItems appends members of one entity type, all credited to addedBy.
// Contributor count is DISTINCT added_by_user_id, so calling this twice with
// two different users is how a collection comes to have two builders.
func (suite *SceneServiceIntegrationTestSuite) addItems(
	c *communitym.Collection, addedBy uint, entityType string, entityIDs ...uint,
) {
	for i, id := range entityIDs {
		suite.Require().NoError(suite.db.Create(&communitym.CollectionItem{
			CollectionID:  c.ID,
			EntityType:    entityType,
			EntityID:      id,
			Position:      i,
			AddedByUserID: addedBy,
		}).Error)
	}
}

// fillerReleaseIDs returns n synthetic release ids. collection_items.entity_id
// has no foreign key, so these need no release rows — and that is faithful to
// production, where a member can outlive the entity it points at. They exist to
// pad the DENOMINATOR: a release can never be scene-local under the rule.
func fillerReleaseIDs(n int) []uint {
	ids := make([]uint, n)
	for i := range ids {
		ids[i] = uint(i + 1)
	}
	return ids
}

// collectionTitles flattens a result set to titles, preserving rank order.
func collectionTitles(rows []contracts.SceneCollectionSummary) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Title
	}
	return out
}

// seedPhoenixRooms clears the scene existence gate (sceneMinVenues) and returns
// the two rooms, which double as venue members in the mixed-type fixtures.
func (suite *SceneServiceIntegrationTestSuite) seedPhoenixRooms() (roomA, roomB uint) {
	return suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ").ID,
		suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ").ID
}

// =============================================================================
// THE RULE, ON A DENSE SCENE
// =============================================================================

// Three collections qualify by different arms of the rule and three are
// excluded for three different reasons, asserted in one pass: an implementation
// that widened the rule cannot pass by also narrowing the ordering, and one
// that broke the ordering cannot pass by also dropping a qualifier.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCollections_RanksQualifiersAndExcludesTheRest() {
	roomA, roomB := suite.seedPhoenixRooms()
	curator := suite.createUser()
	coCurator := suite.createUser()

	local := make([]uint, 0, 6)
	for i := 0; i < 6; i++ {
		local = append(local, suite.createArtist(fmt.Sprintf("Phoenix Band %d", i)).ID)
	}
	austin := suite.createArtistIn("Austin Band", "Austin", "TX").ID
	show := suite.createApprovedShow("a phoenix booking", roomA, local[0], curator.ID,
		time.Now().UTC().AddDate(0, 0, 7))

	// QUALIFIES by the ratio arm: 3 of 4 members local (75%), one builder.
	ratioArm := suite.createCollection("Phoenix Heavy", curator.ID, true)
	suite.addItems(ratioArm, curator.ID, communitym.CollectionEntityArtist,
		local[0], local[1], local[2], austin)

	// QUALIFIES by the absolute arm: 5 of 25 local (20%, well under the ratio)
	// — five is the threshold, so a genuinely city-heavy big list survives its
	// long tail of releases.
	absoluteArm := suite.createCollection("Desert Comp", curator.ID, true)
	suite.addItems(absoluteArm, curator.ID, communitym.CollectionEntityArtist,
		local[0], local[1], local[2], local[3], local[4])
	suite.addItems(absoluteArm, curator.ID, communitym.CollectionEntityRelease, fillerReleaseIDs(20)...)

	// QUALIFIES: venues and shows are scene-local too, not only artists. 3 of 3
	// local, and TWO builders — which is what breaks its tie with Phoenix Heavy
	// at three scene-local members apiece.
	mixedTypes := suite.createCollection("Rooms of Phoenix", curator.ID, true)
	suite.addItems(mixedTypes, curator.ID, communitym.CollectionEntityVenue, roomA, roomB)
	suite.addItems(mixedTypes, coCurator.ID, communitym.CollectionEntityShow, show.ID)

	// EXCLUDED — fails both arms: 4 of 24 local (16.7% < 50%, and 4 < 5).
	underBoth := suite.createCollection("National Comp", curator.ID, true)
	suite.addItems(underBoth, curator.ID, communitym.CollectionEntityArtist,
		local[0], local[1], local[2], local[3])
	suite.addItems(underBoth, curator.ID, communitym.CollectionEntityRelease, fillerReleaseIDs(20)...)

	// EXCLUDED — nothing local at all.
	elsewhere := suite.createCollection("Austin Only", curator.ID, true)
	suite.addItems(elsewhere, curator.ID, communitym.CollectionEntityArtist, austin)

	// EXCLUDED — empty collections have no members to be local.
	suite.createCollection("Empty Shelf", curator.ID, true)

	got, err := suite.sceneService.GetSceneCollections("Phoenix", "AZ", 5)
	suite.Require().NoError(err)

	suite.Equal(
		[]string{"Desert Comp", "Rooms of Phoenix", "Phoenix Heavy"},
		collectionTitles(got),
		"ranked by scene-local count desc; the 3-3 tie breaks on builder count",
	)

	suite.Require().Len(got, 3)

	suite.Equal(5, got[0].SceneLocalItemCount, "Desert Comp: five local artists")
	suite.Equal(25, got[0].ItemCount, "Desert Comp: releases still count toward the total")
	suite.Equal(1, got[0].ContributorCount)

	suite.Equal(3, got[1].SceneLocalItemCount, "Rooms of Phoenix: two venues plus one show")
	suite.Equal(3, got[1].ItemCount)
	suite.Equal(2, got[1].ContributorCount, "two distinct added_by_user_id values")

	suite.Equal(3, got[2].SceneLocalItemCount, "Phoenix Heavy: three of four artists")
	suite.Equal(4, got[2].ItemCount)
	suite.Equal(1, got[2].ContributorCount)
}

// The 50% boundary is inclusive and computed in integer arithmetic, so exactly
// half qualifies. The near-miss fixture differs by ONE non-local member, which
// is the smallest change that must flip the answer.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCollections_ExactlyHalfQualifiesOneShortDoesNot() {
	suite.seedPhoenixRooms()
	curator := suite.createUser()

	local := []uint{
		suite.createArtist("Phoenix A").ID,
		suite.createArtist("Phoenix B").ID,
	}
	austin := []uint{
		suite.createArtistIn("Austin A", "Austin", "TX").ID,
		suite.createArtistIn("Austin B", "Austin", "TX").ID,
		suite.createArtistIn("Austin C", "Austin", "TX").ID,
	}

	// 2 of 4 == exactly 50%, and 2 < 5, so only the ratio arm can admit it.
	half := suite.createCollection("Half Local", curator.ID, true)
	suite.addItems(half, curator.ID, communitym.CollectionEntityArtist,
		local[0], local[1], austin[0], austin[1])

	// 2 of 5 == 40%. One extra out-of-town member is the whole difference.
	underHalf := suite.createCollection("Just Under", curator.ID, true)
	suite.addItems(underHalf, curator.ID, communitym.CollectionEntityArtist,
		local[0], local[1], austin[0], austin[1], austin[2])

	got, err := suite.sceneService.GetSceneCollections("Phoenix", "AZ", 5)
	suite.Require().NoError(err)

	suite.Equal([]string{"Half Local"}, collectionTitles(got),
		"exactly half qualifies; one member under half does not")
}

// A show member counts as scene-local only when it is APPROVED — the same
// definition of "this scene's shows" every other scene surface uses. Pending
// submissions are invisible to readers, so counting them would let anyone push
// a collection onto the rail with bookings nobody can see.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCollections_OnlyApprovedShowsAreSceneLocal() {
	roomA, _ := suite.seedPhoenixRooms()
	curator := suite.createUser()
	band := suite.createArtist("Phoenix Band").ID
	austin := suite.createArtistIn("Austin Band", "Austin", "TX").ID

	approved := suite.createApprovedShow("published booking", roomA, band, curator.ID,
		time.Now().UTC().AddDate(0, 0, 7))
	pending := suite.createPendingShow("unreviewed submission", roomA, band, curator.ID,
		time.Now().UTC().AddDate(0, 0, 8))

	// 2 of 4 local (the approved show + the band) == exactly half, so it
	// qualifies on the strength of the approved show alone.
	withApproved := suite.createCollection("Counts The Approved Show", curator.ID, true)
	suite.addItems(withApproved, curator.ID, communitym.CollectionEntityArtist, band, austin)
	suite.addItems(withApproved, curator.ID, communitym.CollectionEntityShow, approved.ID)
	suite.addItems(withApproved, curator.ID, communitym.CollectionEntityRelease, 1)

	// Identical shape, but the show is PENDING: 1 of 4 local, so it falls out.
	// The only difference between the two fixtures is the show's status.
	withPending := suite.createCollection("Ignores The Pending Show", curator.ID, true)
	suite.addItems(withPending, curator.ID, communitym.CollectionEntityArtist, band, austin)
	suite.addItems(withPending, curator.ID, communitym.CollectionEntityShow, pending.ID)
	suite.addItems(withPending, curator.ID, communitym.CollectionEntityRelease, 1)

	got, err := suite.sceneService.GetSceneCollections("Phoenix", "AZ", 5)
	suite.Require().NoError(err)

	suite.Equal([]string{"Counts The Approved Show"}, collectionTitles(got),
		"show status is the only difference between the two fixtures")
	suite.Require().Len(got, 1)
	suite.Equal(2, got[0].SceneLocalItemCount, "the band and the approved show")
	suite.Equal(4, got[0].ItemCount)
}

// The tripwire for a seventh collection entity type. The rule splits every
// member type into "can be scene-local" and "counts only toward the total",
// and a new type that nobody classifies would silently join the DENOMINATOR:
// every collection holding one loses ratio, the rail quietly reshuffles, and
// nothing fails. Iterating the single enumeration in models/community is what
// turns that into a failing test at the moment the type is added.
func TestSceneCollections_EveryEntityTypeIsClassified(t *testing.T) {
	canBeSceneLocal := map[string]bool{
		communitym.CollectionEntityArtist: true,
		communitym.CollectionEntityVenue:  true,
		communitym.CollectionEntityShow:   true,
	}
	// Deliberately not locatable: a release, label or festival has no home
	// scene the rule is willing to assert (a label's roster spans cities, a
	// release belongs to its artist, a festival is its own venue-less thing).
	countsOnlyTowardTotal := map[string]bool{
		communitym.CollectionEntityRelease:  true,
		communitym.CollectionEntityLabel:    true,
		communitym.CollectionEntityFestival: true,
	}

	for _, entityType := range communitym.AllCollectionEntityTypes {
		if !canBeSceneLocal[entityType] && !countsOnlyTowardTotal[entityType] {
			t.Errorf("collection entity type %q is neither scene-local nor explicitly excluded. "+
				"Decide which it is in GetSceneCollections (and add it to the CTE if it can be "+
				"located) before shipping it, or it silently dilutes every collection's ratio.",
				entityType)
		}
	}
}

// =============================================================================
// PRIVACY
// =============================================================================

// A private collection must never surface, no matter how local it is. The test
// proves the EXCLUSION IS CAUSED BY is_public by flipping only that column and
// re-running: if the collection were being dropped for some unrelated seeding
// mistake, the second half would fail too, and the test could not pass by
// accident.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCollections_NeverSurfacesPrivateCollections() {
	suite.seedPhoenixRooms()
	owner := suite.createUser()

	local := make([]uint, 0, 6)
	for i := 0; i < 6; i++ {
		local = append(local, suite.createArtist(fmt.Sprintf("Phoenix Band %d", i)).ID)
	}

	// 6 of 6 local — qualifies under BOTH arms, so nothing but visibility can
	// be keeping it out.
	secret := suite.createCollection("Private Phoenix List", owner.ID, false)
	suite.addItems(secret, owner.ID, communitym.CollectionEntityArtist, local...)

	got, err := suite.sceneService.GetSceneCollections("Phoenix", "AZ", 5)
	suite.Require().NoError(err)
	suite.Empty(collectionTitles(got), "a private collection must not reach a public rail")

	// Flip ONLY is_public. The same rows now qualify.
	suite.Require().NoError(suite.db.Model(secret).Update("is_public", true).Error)

	got, err = suite.sceneService.GetSceneCollections("Phoenix", "AZ", 5)
	suite.Require().NoError(err)
	suite.Equal([]string{"Private Phoenix List"}, collectionTitles(got),
		"visibility was the only thing excluding it, so the first assertion tested privacy")
}

// The roster rule's most fragile arm (PSY-1237): a band with a NULL metro whose
// home city is a CBSA MEMBER place (Mesa, Scottsdale -> Phoenix) is still based
// in the scene. Every other fixture in this file seeds a non-null metro, so
// without this test a regression that dropped the member-place OR-branch would
// leave the whole file green while the rail under-counted every metro scene
// that has suburb-based bands — which is most of them.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCollections_CountsNullMetroMemberPlaceBands() {
	suite.seedPhoenixRooms()
	curator := suite.createUser()

	mesa := suite.createArtistInNullMetro("Mesa Band", "Mesa", "AZ").ID
	scottsdale := suite.createArtistInNullMetro("Scottsdale Band", "Scottsdale", "AZ").ID
	// A NULL-metro band in a city that is NOT a Phoenix member place.
	austin := suite.createArtistInNullMetro("Austin Band", "Austin", "TX").ID

	suburbs := suite.createCollection("Valley Suburbs", curator.ID, true)
	suite.addItems(suburbs, curator.ID, communitym.CollectionEntityArtist, mesa, scottsdale, austin)

	got, err := suite.sceneService.GetSceneCollections("Phoenix", "AZ", 5)
	suite.Require().NoError(err)

	suite.Require().Len(got, 1, "suburb bands are based in the metro scene")
	suite.Equal(2, got[0].SceneLocalItemCount,
		"Mesa and Scottsdale are Phoenix CBSA member places; Austin is not")
	suite.Equal(3, got[0].ItemCount)
}

// A member pointing at an entity that no longer exists (collection_items has no
// FK on entity_id) must not match, but must still count toward the total. The
// filler-release fixtures cannot prove this: a release is never scene-local
// whether or not its row exists. This uses an ARTIST id that could have
// matched, which is the only version of the claim worth testing.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCollections_DanglingMemberCountsOnlyInTheTotal() {
	suite.seedPhoenixRooms()
	curator := suite.createUser()

	present := suite.createArtist("Phoenix Band").ID
	deleted := suite.createArtist("Doomed Band")
	deletedID := deleted.ID
	suite.Require().NoError(suite.db.Delete(deleted).Error)

	// 1 of 2 local == exactly half, so it still qualifies — but only because
	// the dangling member is counted honestly in the denominator rather than
	// quietly dropped from it.
	c := suite.createCollection("Half Ghosts", curator.ID, true)
	suite.addItems(c, curator.ID, communitym.CollectionEntityArtist, present, deletedID)

	got, err := suite.sceneService.GetSceneCollections("Phoenix", "AZ", 5)
	suite.Require().NoError(err)

	suite.Require().Len(got, 1)
	suite.Equal(1, got[0].SceneLocalItemCount, "the deleted artist cannot be scene-local")
	suite.Equal(2, got[0].ItemCount, "but it is still a member of the collection")
}

// The ORDER BY must be TOTAL. Under a LIMIT, a non-total order does not merely
// reshuffle the rail — it changes which collections are in it between requests.
// This is the only fixture that ties all the way through scene-local count,
// contributor count AND updated_at, leaving id as the sole discriminator.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCollections_OrderIsTotalWhenEverythingTies() {
	suite.seedPhoenixRooms()
	curator := suite.createUser()
	band := suite.createArtist("Phoenix Band").ID

	first := suite.createCollection("Tie A", curator.ID, true)
	suite.addItems(first, curator.ID, communitym.CollectionEntityArtist, band)
	second := suite.createCollection("Tie B", curator.ID, true)
	suite.addItems(second, curator.ID, communitym.CollectionEntityArtist, band)
	third := suite.createCollection("Tie C", curator.ID, true)
	suite.addItems(third, curator.ID, communitym.CollectionEntityArtist, band)

	// Force updated_at identical: GORM stamps each row at its own Create, so
	// without this the timestamps would break the tie before id ever ran.
	sameInstant := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	for _, c := range []*communitym.Collection{first, second, third} {
		suite.Require().NoError(suite.db.Model(c).Update("updated_at", sameInstant).Error)
	}

	// Ascending id is the last key, so the creation order is the rail order.
	want := []string{"Tie A", "Tie B", "Tie C"}
	for i := 0; i < 3; i++ {
		got, err := suite.sceneService.GetSceneCollections("Phoenix", "AZ", 2)
		suite.Require().NoError(err)
		suite.Equal(want[:2], collectionTitles(got),
			"a fully-tied set must return the SAME two collections on every request")
	}
}

// =============================================================================
// SPARSE SCENE / BOUNDS
// =============================================================================

// A real scene with no qualifying collection answers with an empty list, not an
// error — the rail hides itself. Seeded alongside a dense Phoenix whose
// collections must NOT bleed across the metro boundary.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCollections_SparseSceneReturnsEmpty() {
	suite.seedPhoenixRooms()
	suite.createVerifiedVenue("Club Congress", "Tucson", "AZ")
	suite.createVerifiedVenue("191 Toole", "Tucson", "AZ")
	curator := suite.createUser()

	phoenixBands := []uint{
		suite.createArtist("Phoenix A").ID,
		suite.createArtist("Phoenix B").ID,
		suite.createArtist("Phoenix C").ID,
	}
	phoenixOnly := suite.createCollection("Phoenix Only", curator.ID, true)
	suite.addItems(phoenixOnly, curator.ID, communitym.CollectionEntityArtist, phoenixBands...)

	// The collection qualifies for Phoenix...
	phoenix, err := suite.sceneService.GetSceneCollections("Phoenix", "AZ", 5)
	suite.Require().NoError(err)
	suite.Equal([]string{"Phoenix Only"}, collectionTitles(phoenix))

	// ...and for Tucson, a different CBSA, there is nothing at all.
	tucson, err := suite.sceneService.GetSceneCollections("Tucson", "AZ", 5)
	suite.Require().NoError(err)
	suite.Empty(tucson, "a scene with no qualifying collection returns an empty list, not an error")
}

// The no-CBSA fallback branch, which every other test in this file misses:
// Phoenix and Tucson both resolve to metros, so they all exercise the ONE-arg
// artist and venue predicates. The fallback branch binds TWO args each, and
// this query splices both predicates ahead of its own status, entity-type,
// threshold and limit placeholders. A positional-binding slip would land here
// and nowhere else, either erroring outright or counting one city's members
// against another.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCollections_NoCBSAFallbackScene() {
	roomA := suite.createVerifiedVenue("Club One", "Faketown", "ZZ")
	suite.createVerifiedVenue("Club Two", "Faketown", "ZZ")
	suite.Require().Nil(roomA.Metro, "a no-CBSA place has a NULL metro, so the fallback branch is live")
	curator := suite.createUser()

	local := suite.createArtistIn("Faketown Band", "Faketown", "ZZ").ID
	// A band in ANOTHER no-CBSA city must not leak across the (city, state) key.
	elsewhere := suite.createArtistIn("Othertown Band", "Othertown", "ZZ").ID
	show := suite.createApprovedShow("faketown gig", roomA.ID, local, curator.ID,
		time.Now().UTC().AddDate(0, 0, 5))

	// 3 of 4 local: the band, the room, and the show at that room.
	qualifies := suite.createCollection("Faketown Picks", curator.ID, true)
	suite.addItems(qualifies, curator.ID, communitym.CollectionEntityArtist, local, elsewhere)
	suite.addItems(qualifies, curator.ID, communitym.CollectionEntityVenue, roomA.ID)
	suite.addItems(qualifies, curator.ID, communitym.CollectionEntityShow, show.ID)

	// Nothing local to Faketown.
	otherTown := suite.createCollection("Othertown Picks", curator.ID, true)
	suite.addItems(otherTown, curator.ID, communitym.CollectionEntityArtist, elsewhere)

	got, err := suite.sceneService.GetSceneCollections("Faketown", "ZZ", 5)
	suite.Require().NoError(err)

	suite.Equal([]string{"Faketown Picks"}, collectionTitles(got),
		"the fallback branch binds city AND state; a slipped arg would return the wrong set")
	suite.Require().Len(got, 1)
	suite.Equal(3, got[0].SceneLocalItemCount, "band, room, and the show at that room")
	suite.Equal(4, got[0].ItemCount)
}

// A slug that resolves to a place but not to a scene 404s here exactly as it
// does on the sibling rails, rather than answering 200 with an empty rail.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCollections_NotAScene() {
	suite.createVerifiedVenue("The Only Room", "Bisbee", "AZ")

	_, err := suite.sceneService.GetSceneCollections("Bisbee", "AZ", 5)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "scene not found")
}

// The limit is a cap on the rail, applied after ranking.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneCollections_HonorsLimitAfterRanking() {
	suite.seedPhoenixRooms()
	curator := suite.createUser()

	local := make([]uint, 0, 3)
	for i := 0; i < 3; i++ {
		local = append(local, suite.createArtist(fmt.Sprintf("Phoenix Band %d", i)).ID)
	}

	// Three fully-local collections of decreasing size, so rank order is
	// unambiguous and the cap must keep the TOP one.
	big := suite.createCollection("Three Local", curator.ID, true)
	suite.addItems(big, curator.ID, communitym.CollectionEntityArtist, local...)
	mid := suite.createCollection("Two Local", curator.ID, true)
	suite.addItems(mid, curator.ID, communitym.CollectionEntityArtist, local[0], local[1])
	small := suite.createCollection("One Local", curator.ID, true)
	suite.addItems(small, curator.ID, communitym.CollectionEntityArtist, local[0])

	got, err := suite.sceneService.GetSceneCollections("Phoenix", "AZ", 2)
	suite.Require().NoError(err)
	suite.Equal([]string{"Three Local", "Two Local"}, collectionTitles(got))
}
