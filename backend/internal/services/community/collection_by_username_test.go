package community

import (
	"testing"

	"psychic-homily-backend/internal/testutil"
)

// A failed username lookup returns its error rather than an empty page, so the
// handler answers a database failure with its error status instead of a 200
// that reads as a user with no public collections.
func TestGetUserPublicCollectionsByUsername_FailedLookupReturnsError(t *testing.T) {
	svc := NewCollectionService(testutil.UnavailableDB(t))

	collections, total, err := svc.GetUserPublicCollectionsByUsername("nobody", 20, 0)
	if err == nil {
		t.Fatalf("GetUserPublicCollectionsByUsername over a failing database = (%v, %d, nil), want an error", collections, total)
	}
	if collections != nil || total != 0 {
		t.Fatalf("GetUserPublicCollectionsByUsername over a failing database = (%v, %d), want no page", collections, total)
	}
}

// A username no account holds and an account with no public collections both
// answer an empty non-nil page with no error; an account's public collections
// are listed and its private ones are not.
func (suite *CollectionServiceIntegrationTestSuite) TestGetUserPublicCollectionsByUsername_EmptyAndFoundPages() {
	collections, total, err := suite.collectionService.GetUserPublicCollectionsByUsername("no-such-user", 20, 0)
	suite.Require().NoError(err)
	suite.Require().NotNil(collections)
	suite.Empty(collections)
	suite.Zero(total)

	user := suite.createTestUserWithUsername("Owner", "crate-owner")
	suite.createBasicCollection(user, "Private Only")

	collections, total, err = suite.collectionService.GetUserPublicCollectionsByUsername("crate-owner", 20, 0)
	suite.Require().NoError(err)
	suite.Require().NotNil(collections)
	suite.Empty(collections)
	suite.Zero(total)

	public := suite.createPublicCollection(user, "Public Picks")

	collections, total, err = suite.collectionService.GetUserPublicCollectionsByUsername("crate-owner", 20, 0)
	suite.Require().NoError(err)
	suite.Require().Len(collections, 1)
	suite.Equal(public.Slug, collections[0].Slug)
	suite.EqualValues(1, total)
}
