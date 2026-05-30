package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

type EntityExistenceServiceIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *EntityExistenceService
}

func (suite *EntityExistenceServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.svc = NewEntityExistenceService(suite.db)
}

func (suite *EntityExistenceServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *EntityExistenceServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM tag_aliases")
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM festival_artists")
	_, _ = sqlDB.Exec("DELETE FROM festival_venues")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestEntityExistenceServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(EntityExistenceServiceIntegrationTestSuite))
}

func (suite *EntityExistenceServiceIntegrationTestSuite) createEntityExistenceUser() *authm.User {
	user := &authm.User{
		Email:         stringPtr(fmt.Sprintf("entity-existence-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Entity"),
		LastName:      stringPtr("Probe"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(user).Error)
	return user
}

func (suite *EntityExistenceServiceIntegrationTestSuite) TestExists_ShowSlugUsesPublicDetailSemantics() {
	user := suite.createEntityExistenceUser()
	approvedSlug := "approved-show"
	privateSlug := "private-show"

	approved := &catalogm.Show{
		Title:       "Approved Show",
		Slug:        &approvedSlug,
		EventDate:   time.Now().UTC().Add(24 * time.Hour),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	private := &catalogm.Show{
		Title:       "Private Show",
		Slug:        &privateSlug,
		EventDate:   time.Now().UTC().Add(48 * time.Hour),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusPrivate,
		SubmittedBy: &user.ID,
	}
	suite.Require().NoError(suite.db.Create(approved).Error)
	suite.Require().NoError(suite.db.Create(private).Error)

	exists, err := suite.svc.Exists("shows", approvedSlug)
	suite.Require().NoError(err)
	suite.True(exists)

	exists, err = suite.svc.Exists("shows", fmt.Sprintf("%d", approved.ID))
	suite.Require().NoError(err)
	suite.True(exists)

	exists, err = suite.svc.Exists("shows", privateSlug)
	suite.Require().NoError(err)
	suite.False(exists)

	exists, err = suite.svc.Exists("shows", "missing-show")
	suite.Require().NoError(err)
	suite.False(exists)
}

func (suite *EntityExistenceServiceIntegrationTestSuite) TestExists_TagSlugAndID() {
	tag := &catalogm.Tag{Name: "Post Punk", Slug: "post-punk", Category: catalogm.TagCategoryGenre}
	suite.Require().NoError(suite.db.Create(tag).Error)

	exists, err := suite.svc.Exists("tags", "post-punk")
	suite.Require().NoError(err)
	suite.True(exists)

	exists, err = suite.svc.Exists("tags", fmt.Sprintf("%d", tag.ID))
	suite.Require().NoError(err)
	suite.True(exists)

	exists, err = suite.svc.Exists("tags", "missing-tag")
	suite.Require().NoError(err)
	suite.False(exists)
}

func (suite *EntityExistenceServiceIntegrationTestSuite) TestExists_SceneSlugUsesDetailThreshold() {
	venues := []*catalogm.Venue{
		{Name: "Crescent Ballroom", Slug: stringPtr("crescent-ballroom"), City: "Phoenix", State: "AZ", Verified: true},
		{Name: "Valley Bar", Slug: stringPtr("valley-bar"), City: "Phoenix", State: "AZ", Verified: true},
		{Name: "Small Room", Slug: stringPtr("small-room"), City: "Tucson", State: "AZ", Verified: true},
		{Name: "Unverified Room", Slug: stringPtr("unverified-room"), City: "Mesa", State: "AZ", Verified: false},
	}
	for _, venue := range venues {
		suite.Require().NoError(suite.db.Create(venue).Error)
	}
	suite.Require().NoError(suite.db.Model(venues[3]).Update("verified", false).Error)

	exists, err := suite.svc.Exists("scenes", "phoenix-az")
	suite.Require().NoError(err)
	suite.True(exists)

	exists, err = suite.svc.Exists("scenes", "tucson-az")
	suite.Require().NoError(err)
	suite.False(exists)

	exists, err = suite.svc.Exists("scenes", "mesa-az")
	suite.Require().NoError(err)
	suite.False(exists)
}

func (suite *EntityExistenceServiceIntegrationTestSuite) TestExists_UnsupportedEntityTypeIsMissing() {
	exists, err := suite.svc.Exists("collections", "any")
	suite.Require().NoError(err)
	suite.False(exists)
}
