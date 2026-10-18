package engagement

import (
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/testutil"
)

// closeDigestAccount closes u: soft-deleted when softDelete, otherwise
// deactivated.
func (s *CollectionDigestServiceIntegrationSuite) closeDigestAccount(u *authm.User, softDelete bool) {
	if softDelete {
		testutil.SoftDeleteAccount(s.T(), s.db, u.ID)
		return
	}
	testutil.DeactivateAccount(s.T(), s.db, u.ID)
}

// An item added by a deactivated or soft-deleted account still goes out, under
// the label an adder with no public name gets, so the digest names no closed
// account; once the account is reopened the next item names it again.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_ClosedAdderIsNotNamed() {
	for _, softDelete := range []bool{false, true} {
		s.mock.calls = nil
		state := map[bool]string{false: "deactivated", true: "deleted"}[softDelete]
		creator := s.createUser("closed-adder-creator-" + state)
		adder := s.createUser("closed-adder-" + state)
		subscriber := s.createUserWithDigestPref("closed-adder-sub-"+state, true)
		coll := s.createCollection(creator, "Closed Adder "+state, "closed-adder-"+state)
		s.subscribe(coll, subscriber, nil, nil)

		first := s.createArtist("Closed Adder Artist " + coll.Slug)
		s.addItem(coll, adder, communitym.CollectionEntityArtist, first.ID, time.Now().Add(-1*time.Hour))
		s.closeDigestAccount(adder, softDelete)

		s.svc.RunDigestCycleNow()

		require.Len(s.T(), s.mock.calls, 1, "softDelete=%v", softDelete)
		require.Len(s.T(), s.mock.calls[0].Groups, 1)
		g := s.mock.calls[0].Groups[0]
		require.Len(s.T(), g.Items, 1)
		assert.Equal(s.T(), "a contributor", g.Items[0].AddedBy, "softDelete=%v", softDelete)
		assert.NotContains(s.T(), g.Items[0].AddedBy, *adder.Username)
		assert.Equal(s.T(), first.Name, g.Items[0].EntityName, "the item still goes out")

		testutil.ReopenAccount(s.T(), s.db, adder.ID)
		s.mock.calls = nil
		second := s.createArtist("Reopened Adder Artist " + coll.Slug)
		s.addItem(coll, adder, communitym.CollectionEntityArtist, second.ID, time.Now())

		s.svc.RunDigestCycleNow()

		require.Len(s.T(), s.mock.calls, 1, "softDelete=%v", softDelete)
		g = s.mock.calls[0].Groups[0]
		require.Len(s.T(), g.Items, 1)
		assert.Equal(s.T(), *adder.Username, g.Items[0].AddedBy, "softDelete=%v", softDelete)
	}
}

// A collection whose creator is deactivated or soft-deleted is not mailed, so
// the digest carries no link to a collection its detail route refuses; the
// subscriber's other collections still go out, and once the creator is
// reopened the held items go out on the next cycle.
func (s *CollectionDigestServiceIntegrationSuite) TestDigest_ClosedCreatorsCollectionIsNotMailedUntilReopened() {
	live := s.createUser("digest-live-creator")
	deactivated := s.createUser("digest-deactivated-creator")
	deleted := s.createUser("digest-deleted-creator")
	adder := s.createUser("digest-closed-creator-adder")
	subscriber := s.createUserWithDigestPref("digest-closed-creator-sub", true)

	collBy := map[uint]*communitym.Collection{}
	for _, c := range []*authm.User{live, deactivated, deleted} {
		coll := s.createCollection(c, "From "+*c.Username, *c.Username)
		s.subscribe(coll, subscriber, nil, nil)
		a := s.createArtist("Artist for " + *c.Username)
		s.addItem(coll, adder, communitym.CollectionEntityArtist, a.ID, time.Now().Add(-1*time.Hour))
		collBy[c.ID] = coll
	}
	s.closeDigestAccount(deactivated, false)
	s.closeDigestAccount(deleted, true)

	s.svc.RunDigestCycleNow()

	require.Len(s.T(), s.mock.calls, 1)
	require.Len(s.T(), s.mock.calls[0].Groups, 1, "only the live creator's collection is mailed")
	assert.Equal(s.T(), collBy[live.ID].Title, s.mock.calls[0].Groups[0].CollectionTitle)

	testutil.ReopenAccount(s.T(), s.db, deactivated.ID)
	testutil.ReopenAccount(s.T(), s.db, deleted.ID)
	s.mock.calls = nil

	s.svc.RunDigestCycleNow()

	require.Len(s.T(), s.mock.calls, 1)
	titles := make([]string, 0, len(s.mock.calls[0].Groups))
	for _, g := range s.mock.calls[0].Groups {
		titles = append(titles, g.CollectionTitle)
	}
	assert.ElementsMatch(s.T(), []string{collBy[deactivated.ID].Title, collBy[deleted.ID].Title}, titles,
		"the held items go out once the creators are reopened")
}
