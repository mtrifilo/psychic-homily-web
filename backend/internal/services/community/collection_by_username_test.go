package community

import (
	"testing"
	"time"

	authm "psychic-homily-backend/internal/models/auth"
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

// A deactivated or soft-deleted account's public collections answer the empty
// page an unknown username answers, and reactivating or restoring the account
// lists them again.
func (suite *CollectionServiceIntegrationTestSuite) TestGetUserPublicCollectionsByUsername_NonLiveAccountIsEmptyUntilRestored() {
	deactivated := suite.createTestUserWithUsername("Deactivated", "deactivated-crates")
	deleted := suite.createTestUserWithUsername("Deleted", "deleted-crates")
	suite.createPublicCollection(deactivated, "Deactivated Picks")
	suite.createPublicCollection(deleted, "Deleted Picks")

	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", deactivated.ID).
		Update("is_active", false).Error)
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", deleted.ID).
		Updates(map[string]any{"is_active": false, "deleted_at": time.Now()}).Error)

	for _, u := range []*authm.User{deactivated, deleted} {
		collections, total, err := suite.collectionService.GetUserPublicCollectionsByUsername(*u.Username, 20, 0)
		suite.Require().NoError(err)
		suite.Require().NotNil(collections, *u.Username)
		suite.Empty(collections, *u.Username)
		suite.Zero(total, *u.Username)
	}

	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", deactivated.ID).
		Update("is_active", true).Error)
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", deleted.ID).
		Updates(map[string]any{"is_active": true, "deleted_at": nil}).Error)

	for _, u := range []*authm.User{deactivated, deleted} {
		collections, total, err := suite.collectionService.GetUserPublicCollectionsByUsername(*u.Username, 20, 0)
		suite.Require().NoError(err)
		suite.Len(collections, 1, *u.Username)
		suite.EqualValues(1, total, *u.Username)
	}
}
