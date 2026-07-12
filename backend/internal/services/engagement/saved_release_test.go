package engagement

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/testutil"
)

type SavedReleaseServiceIntegrationTestSuite struct {
	suite.Suite
	testDB         *testutil.TestDatabase
	db             *gorm.DB
	releaseService *catalog.ReleaseService
	savedRelease   *SavedReleaseService
}

func (suite *SavedReleaseServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.releaseService = catalog.NewReleaseService(suite.db)
	suite.savedRelease = NewSavedReleaseService(suite.db, suite.releaseService)
}

func (suite *SavedReleaseServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *SavedReleaseServiceIntegrationTestSuite) TearDownTest() {
	for _, table := range []string{
		"user_bookmarks", "artist_releases", "release_labels", "releases", "labels", "artists", "users",
	} {
		suite.Require().NoError(suite.db.Exec("DELETE FROM " + table).Error)
	}
}

func TestSavedReleaseServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(SavedReleaseServiceIntegrationTestSuite))
}

func (suite *SavedReleaseServiceIntegrationTestSuite) createUser(index int) *authm.User {
	email := fmt.Sprintf("saved-release-%d@example.com", index)
	user := &authm.User{Email: &email, IsActive: true, EmailVerified: true}
	suite.Require().NoError(suite.db.Create(user).Error)
	return user
}

func (suite *SavedReleaseServiceIntegrationTestSuite) createRelease(title string) *catalogm.Release {
	slug := fmt.Sprintf("%s-%d", title, time.Now().UnixNano())
	release := &catalogm.Release{Title: title, Slug: &slug, ReleaseType: catalogm.ReleaseTypeLP}
	suite.Require().NoError(suite.db.Create(release).Error)
	return release
}

func (suite *SavedReleaseServiceIntegrationTestSuite) TestSaveRelease_IdempotentAndCounted() {
	user := suite.createUser(1)
	release := suite.createRelease("Saved Once")

	suite.Require().NoError(suite.savedRelease.SaveRelease(user.ID, release.ID))
	suite.Require().NoError(suite.savedRelease.SaveRelease(user.ID, release.ID))

	count, err := suite.savedRelease.GetSaveCount(release.ID)
	suite.Require().NoError(err)
	suite.Equal(1, count)
	isSaved, err := suite.savedRelease.IsReleaseSaved(user.ID, release.ID)
	suite.Require().NoError(err)
	suite.True(isSaved)
}

func (suite *SavedReleaseServiceIntegrationTestSuite) TestSaveRelease_RejectsUnknownRelease() {
	err := suite.savedRelease.SaveRelease(suite.createUser(2).ID, 999999)
	suite.Require().Error(err)
	var releaseErr *apperrors.ReleaseError
	suite.Require().True(errors.As(err, &releaseErr))
	suite.Equal(apperrors.CodeReleaseNotFound, releaseErr.Code)
}

func (suite *SavedReleaseServiceIntegrationTestSuite) TestBatchCounts_ZeroFillsAndUsesBookmarkAction() {
	userA := suite.createUser(3)
	userB := suite.createUser(4)
	saved := suite.createRelease("Popular")
	unsaved := suite.createRelease("Quiet")

	suite.Require().NoError(suite.savedRelease.SaveRelease(userA.ID, saved.ID))
	suite.Require().NoError(suite.savedRelease.SaveRelease(userB.ID, saved.ID))
	// A different action on the same entity must not inflate Save.
	suite.Require().NoError(suite.db.Create(&engagementm.UserBookmark{
		UserID: userA.ID, EntityType: engagementm.BookmarkEntityRelease,
		EntityID: saved.ID, Action: engagementm.BookmarkActionFollow, CreatedAt: time.Now().UTC(),
	}).Error)

	counts, err := suite.savedRelease.GetBatchSaveCounts([]uint{saved.ID, unsaved.ID})
	suite.Require().NoError(err)
	suite.Equal(2, counts[saved.ID])
	suite.Equal(0, counts[unsaved.ID])

	savedIDs, err := suite.savedRelease.GetSavedReleaseIDs(userA.ID, []uint{saved.ID, unsaved.ID})
	suite.Require().NoError(err)
	suite.True(savedIDs[saved.ID])
	suite.False(savedIDs[unsaved.ID])
}

func (suite *SavedReleaseServiceIntegrationTestSuite) TestSavedList_PreservesSaveOrderAndHydratesLinks() {
	user := suite.createUser(5)
	older := suite.createRelease("Older Save")
	newer := suite.createRelease("Newer Save")
	artistSlug := "linked-artist"
	artist := &catalogm.Artist{Name: "Linked Artist", Slug: &artistSlug}
	suite.Require().NoError(suite.db.Create(artist).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ArtistRelease{
		ArtistID: artist.ID, ReleaseID: newer.ID, Role: catalogm.ArtistReleaseRoleMain,
	}).Error)
	labelSlug := "linked-label"
	label := &catalogm.Label{Name: "Linked Label", Slug: &labelSlug}
	suite.Require().NoError(suite.db.Create(label).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ReleaseLabel{ReleaseID: newer.ID, LabelID: label.ID}).Error)

	suite.Require().NoError(suite.savedRelease.SaveRelease(user.ID, newer.ID))
	suite.Require().NoError(suite.savedRelease.SaveRelease(user.ID, older.ID))
	suite.Require().NoError(suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityRelease, older.ID, engagementm.BookmarkActionReleaseSave).
		Update("created_at", time.Now().UTC().Add(-time.Hour)).Error)

	releases, total, err := suite.savedRelease.GetUserSavedReleases(user.ID, 50, 0)
	suite.Require().NoError(err)
	suite.EqualValues(2, total)
	suite.Require().Len(releases, 2)
	suite.Equal(newer.ID, releases[0].ID)
	suite.Require().Len(releases[0].Artists, 1)
	suite.Equal("linked-artist", releases[0].Artists[0].Slug)
	suite.Require().NotNil(releases[0].LabelSlug)
	suite.Equal("linked-label", *releases[0].LabelSlug)
}

func (suite *SavedReleaseServiceIntegrationTestSuite) TestUnsaveRelease_RemovesSavedState() {
	user := suite.createUser(6)
	release := suite.createRelease("Remove Me")
	suite.Require().NoError(suite.savedRelease.SaveRelease(user.ID, release.ID))
	suite.Require().NoError(suite.savedRelease.UnsaveRelease(user.ID, release.ID))
	isSaved, err := suite.savedRelease.IsReleaseSaved(user.ID, release.ID)
	suite.Require().NoError(err)
	suite.False(isSaved)
}

func (suite *SavedReleaseServiceIntegrationTestSuite) TestDeleteRelease_CleansPolymorphicBookmarks() {
	user := suite.createUser(7)
	release := suite.createRelease("Delete Me")
	suite.Require().NoError(suite.savedRelease.SaveRelease(user.ID, release.ID))
	suite.Require().NoError(suite.releaseService.DeleteRelease(release.ID))

	var count int64
	suite.Require().NoError(suite.db.Model(&engagementm.UserBookmark{}).
		Where("entity_type = ? AND entity_id = ?", engagementm.BookmarkEntityRelease, release.ID).
		Count(&count).Error)
	suite.Zero(count)
}

func (suite *SavedReleaseServiceIntegrationTestSuite) TestSaveAndDeleteRelease_NeverLeavesDanglingBookmark() {
	user := suite.createUser(8)
	for i := 0; i < 20; i++ {
		release := suite.createRelease(fmt.Sprintf("Concurrent %d", i))
		start := make(chan struct{})
		var saveErr, deleteErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			saveErr = suite.savedRelease.SaveRelease(user.ID, release.ID)
		}()
		go func() {
			defer wg.Done()
			<-start
			deleteErr = suite.releaseService.DeleteRelease(release.ID)
		}()
		close(start)
		wg.Wait()

		suite.Require().NoError(deleteErr)
		if saveErr != nil {
			var releaseErr *apperrors.ReleaseError
			suite.Require().True(errors.As(saveErr, &releaseErr))
			suite.Equal(apperrors.CodeReleaseNotFound, releaseErr.Code)
		}
	}

	var dangling int64
	suite.Require().NoError(suite.db.Model(&engagementm.UserBookmark{}).
		Where("entity_type = ?", engagementm.BookmarkEntityRelease).
		Count(&dangling).Error)
	suite.Zero(dangling)
}
