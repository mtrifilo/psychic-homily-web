package catalog

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type LabelServiceIntegrationTestSuite struct {
	suite.Suite
	testDB         *testutil.TestDatabase
	db             *gorm.DB
	labelService   *LabelService
	artistService  *ArtistService
	releaseService *ReleaseService
}

func (suite *LabelServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.labelService = &LabelService{db: suite.testDB.DB}
	suite.artistService = &ArtistService{db: suite.testDB.DB}
	suite.releaseService = &ReleaseService{db: suite.testDB.DB}
}

func (suite *LabelServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

// TearDownTest cleans up data between tests for isolation
func (suite *LabelServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM release_labels")
	_, _ = sqlDB.Exec("DELETE FROM artist_labels")
	_, _ = sqlDB.Exec("DELETE FROM labels")
	_, _ = sqlDB.Exec("DELETE FROM release_external_links")
	_, _ = sqlDB.Exec("DELETE FROM artist_releases")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

func TestLabelServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(LabelServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *LabelServiceIntegrationTestSuite) createTestLabel(name string) *contracts.LabelDetailResponse {
	resp, err := suite.labelService.CreateLabel(&contracts.CreateLabelRequest{Name: name})
	suite.Require().NoError(err)
	return resp
}

func (suite *LabelServiceIntegrationTestSuite) createTestArtistForLabel(name string) *catalogm.Artist {
	artist := &catalogm.Artist{
		Name: name,
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *LabelServiceIntegrationTestSuite) createTestReleaseForLabel(title string) *catalogm.Release {
	slug := title
	release := &catalogm.Release{
		Title: title,
		Slug:  &slug,
	}
	err := suite.db.Create(release).Error
	suite.Require().NoError(err)
	return release
}

// =============================================================================
// Group 1: CreateLabel
// =============================================================================

func (suite *LabelServiceIntegrationTestSuite) TestCreateLabel_Success() {
	city := "Seattle"
	state := "WA"
	year := 1988
	req := &contracts.CreateLabelRequest{
		Name:        "Sub Pop",
		City:        &city,
		State:       &state,
		FoundedYear: &year,
	}

	resp, err := suite.labelService.CreateLabel(req)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal("Sub Pop", resp.Name)
	suite.Equal("sub-pop", resp.Slug)
	suite.Equal("Seattle", *resp.City)
	suite.Equal("WA", *resp.State)
	suite.Equal(1988, *resp.FoundedYear)
	suite.Equal("active", resp.Status)
	suite.Equal(0, resp.ArtistCount)
	suite.Equal(0, resp.ReleaseCount)
}

func (suite *LabelServiceIntegrationTestSuite) TestCreateLabel_DefaultStatus() {
	req := &contracts.CreateLabelRequest{
		Name: "Mystery Label",
	}

	resp, err := suite.labelService.CreateLabel(req)

	suite.Require().NoError(err)
	suite.Equal("active", resp.Status)
}

func (suite *LabelServiceIntegrationTestSuite) TestCreateLabel_CustomStatus() {
	req := &contracts.CreateLabelRequest{
		Name:   "Old Label",
		Status: "defunct",
	}

	resp, err := suite.labelService.CreateLabel(req)

	suite.Require().NoError(err)
	suite.Equal("defunct", resp.Status)
}

func (suite *LabelServiceIntegrationTestSuite) TestCreateLabel_WithSocial() {
	website := "https://subpop.com"
	instagram := "subpop"
	req := &contracts.CreateLabelRequest{
		Name:      "Social Label",
		Website:   &website,
		Instagram: &instagram,
	}

	resp, err := suite.labelService.CreateLabel(req)

	suite.Require().NoError(err)
	suite.Equal("https://subpop.com", *resp.Social.Website)
	suite.Equal("subpop", *resp.Social.Instagram)
}

func (suite *LabelServiceIntegrationTestSuite) TestCreateLabel_UniqueSlug() {
	req1 := &contracts.CreateLabelRequest{Name: "Merge Records"}
	resp1, err := suite.labelService.CreateLabel(req1)
	suite.Require().NoError(err)

	req2 := &contracts.CreateLabelRequest{Name: "Merge Records"}
	resp2, err := suite.labelService.CreateLabel(req2)
	suite.Require().NoError(err)

	suite.NotEqual(resp1.Slug, resp2.Slug)
	suite.Equal("merge-records", resp1.Slug)
	suite.Equal("merge-records-2", resp2.Slug)
}

// =============================================================================
// Group 2: GetLabel / GetLabelBySlug
// =============================================================================

func (suite *LabelServiceIntegrationTestSuite) TestGetLabel_Success() {
	created := suite.createTestLabel("Get Test Label")

	resp, err := suite.labelService.GetLabel(created.ID)

	suite.Require().NoError(err)
	suite.Equal(created.ID, resp.ID)
	suite.Equal("Get Test Label", resp.Name)
}

func (suite *LabelServiceIntegrationTestSuite) TestGetLabel_NotFound() {
	resp, err := suite.labelService.GetLabel(99999)

	suite.Require().Error(err)
	suite.Nil(resp)
	var labelErr *apperrors.LabelError
	suite.ErrorAs(err, &labelErr)
	suite.Equal(apperrors.CodeLabelNotFound, labelErr.Code)
}

func (suite *LabelServiceIntegrationTestSuite) TestGetLabelBySlug_Success() {
	created := suite.createTestLabel("Slug Test Label")

	resp, err := suite.labelService.GetLabelBySlug(created.Slug)

	suite.Require().NoError(err)
	suite.Equal(created.ID, resp.ID)
	suite.Equal(created.Slug, resp.Slug)
}

func (suite *LabelServiceIntegrationTestSuite) TestGetLabelBySlug_NotFound() {
	resp, err := suite.labelService.GetLabelBySlug("nonexistent-slug-xyz")

	suite.Require().Error(err)
	suite.Nil(resp)
	var labelErr *apperrors.LabelError
	suite.ErrorAs(err, &labelErr)
	suite.Equal(apperrors.CodeLabelNotFound, labelErr.Code)
}

// =============================================================================
// Group 3: ListLabels
// =============================================================================

func (suite *LabelServiceIntegrationTestSuite) TestListLabels_All() {
	suite.labelService.CreateLabel(&contracts.CreateLabelRequest{Name: "Alpha Records"})
	suite.labelService.CreateLabel(&contracts.CreateLabelRequest{Name: "Beta Records"})
	suite.labelService.CreateLabel(&contracts.CreateLabelRequest{Name: "Charlie Records"})

	resp, err := suite.labelService.ListLabels(map[string]interface{}{})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 3)
	// Ordered by name ASC
	suite.Equal("Alpha Records", resp[0].Name)
	suite.Equal("Beta Records", resp[1].Name)
	suite.Equal("Charlie Records", resp[2].Name)
}

func (suite *LabelServiceIntegrationTestSuite) TestListLabels_FilterByStatus() {
	suite.labelService.CreateLabel(&contracts.CreateLabelRequest{Name: "Active Label", Status: "active"})
	suite.labelService.CreateLabel(&contracts.CreateLabelRequest{Name: "Defunct Label", Status: "defunct"})

	resp, err := suite.labelService.ListLabels(map[string]interface{}{"status": "defunct"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Defunct Label", resp[0].Name)
}

func (suite *LabelServiceIntegrationTestSuite) TestListLabels_FilterByCity() {
	city1 := "Seattle"
	city2 := "Portland"
	suite.labelService.CreateLabel(&contracts.CreateLabelRequest{Name: "Seattle Label", City: &city1})
	suite.labelService.CreateLabel(&contracts.CreateLabelRequest{Name: "Portland Label", City: &city2})

	resp, err := suite.labelService.ListLabels(map[string]interface{}{"city": "Seattle"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Seattle Label", resp[0].Name)
}

func (suite *LabelServiceIntegrationTestSuite) TestListLabels_FilterByState() {
	state1 := "WA"
	state2 := "OR"
	suite.labelService.CreateLabel(&contracts.CreateLabelRequest{Name: "WA Label", State: &state1})
	suite.labelService.CreateLabel(&contracts.CreateLabelRequest{Name: "OR Label", State: &state2})

	resp, err := suite.labelService.ListLabels(map[string]interface{}{"state": "WA"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("WA Label", resp[0].Name)
}

func (suite *LabelServiceIntegrationTestSuite) TestListLabels_ArtistAndReleaseCounts() {
	label := suite.createTestLabel("Counted Label")

	artist := suite.createTestArtistForLabel("Label Artist")
	suite.db.Exec("INSERT INTO artist_labels (artist_id, label_id) VALUES (?, ?)", artist.ID, label.ID)

	release := suite.createTestReleaseForLabel("Label Release")
	suite.db.Exec("INSERT INTO release_labels (release_id, label_id) VALUES (?, ?)", release.ID, label.ID)

	resp, err := suite.labelService.ListLabels(map[string]interface{}{})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal(1, resp[0].ArtistCount)
	suite.Equal(1, resp[0].ReleaseCount)
}

// =============================================================================
// Group 4: UpdateLabel
// =============================================================================

func (suite *LabelServiceIntegrationTestSuite) TestUpdateLabel_BasicFields() {
	created := suite.createTestLabel("Original Label")

	newName := "Updated Label"
	newCity := "Portland"
	resp, err := suite.labelService.UpdateLabel(created.ID, &contracts.UpdateLabelRequest{
		Name: &newName,
		City: &newCity,
	})

	suite.Require().NoError(err)
	suite.Equal("Updated Label", resp.Name)
	suite.Equal("Portland", *resp.City)
}

func (suite *LabelServiceIntegrationTestSuite) TestUpdateLabel_NameChangeRegeneratesSlug() {
	created := suite.createTestLabel("Old Label Name")
	oldSlug := created.Slug

	newName := "New Label Name"
	resp, err := suite.labelService.UpdateLabel(created.ID, &contracts.UpdateLabelRequest{
		Name: &newName,
	})

	suite.Require().NoError(err)
	suite.Equal("New Label Name", resp.Name)
	suite.NotEqual(oldSlug, resp.Slug)
	suite.Equal("new-label-name", resp.Slug)
}

func (suite *LabelServiceIntegrationTestSuite) TestUpdateLabel_NotFound() {
	newName := "Anything"
	resp, err := suite.labelService.UpdateLabel(99999, &contracts.UpdateLabelRequest{Name: &newName})

	suite.Require().Error(err)
	suite.Nil(resp)
	var labelErr *apperrors.LabelError
	suite.ErrorAs(err, &labelErr)
	suite.Equal(apperrors.CodeLabelNotFound, labelErr.Code)
}

func (suite *LabelServiceIntegrationTestSuite) TestUpdateLabel_NoChanges() {
	created := suite.createTestLabel("Stable Label")

	resp, err := suite.labelService.UpdateLabel(created.ID, &contracts.UpdateLabelRequest{})

	suite.Require().NoError(err)
	suite.Equal("Stable Label", resp.Name)
}

func (suite *LabelServiceIntegrationTestSuite) TestUpdateLabel_SocialFields() {
	created := suite.createTestLabel("Social Update Label")

	instagram := "newhandle"
	website := "https://newsite.com"
	resp, err := suite.labelService.UpdateLabel(created.ID, &contracts.UpdateLabelRequest{
		Instagram: &instagram,
		Website:   &website,
	})

	suite.Require().NoError(err)
	suite.Equal("newhandle", *resp.Social.Instagram)
	suite.Equal("https://newsite.com", *resp.Social.Website)
}

// =============================================================================
// Group 5: DeleteLabel
// =============================================================================

func (suite *LabelServiceIntegrationTestSuite) TestDeleteLabel_Success() {
	created := suite.createTestLabel("Delete Me Label")

	err := suite.labelService.DeleteLabel(created.ID)

	suite.Require().NoError(err)

	// Verify it's gone
	_, err = suite.labelService.GetLabel(created.ID)
	suite.Error(err)
}

func (suite *LabelServiceIntegrationTestSuite) TestDeleteLabel_NotFound() {
	err := suite.labelService.DeleteLabel(99999)

	suite.Require().Error(err)
	var labelErr *apperrors.LabelError
	suite.ErrorAs(err, &labelErr)
	suite.Equal(apperrors.CodeLabelNotFound, labelErr.Code)
}

func (suite *LabelServiceIntegrationTestSuite) TestDeleteLabel_CascadesJunctions() {
	created := suite.createTestLabel("Cascade Label")
	artist := suite.createTestArtistForLabel("Cascade Artist")
	release := suite.createTestReleaseForLabel("Cascade Release")

	suite.db.Exec("INSERT INTO artist_labels (artist_id, label_id) VALUES (?, ?)", artist.ID, created.ID)
	suite.db.Exec("INSERT INTO release_labels (release_id, label_id) VALUES (?, ?)", release.ID, created.ID)

	err := suite.labelService.DeleteLabel(created.ID)
	suite.Require().NoError(err)

	// Verify artist_labels cleaned up
	var alCount int64
	suite.db.Table("artist_labels").Where("label_id = ?", created.ID).Count(&alCount)
	suite.Equal(int64(0), alCount)

	// Verify release_labels cleaned up
	var rlCount int64
	suite.db.Table("release_labels").Where("label_id = ?", created.ID).Count(&rlCount)
	suite.Equal(int64(0), rlCount)
}

// =============================================================================
// Group 6: GetLabelRoster
// =============================================================================

func (suite *LabelServiceIntegrationTestSuite) TestGetLabelRoster_Success() {
	label := suite.createTestLabel("Roster Label")
	artist1 := suite.createTestArtistForLabel("Artist Alpha")
	artist2 := suite.createTestArtistForLabel("Artist Beta")

	suite.db.Exec("INSERT INTO artist_labels (artist_id, label_id) VALUES (?, ?)", artist1.ID, label.ID)
	suite.db.Exec("INSERT INTO artist_labels (artist_id, label_id) VALUES (?, ?)", artist2.ID, label.ID)

	resp, err := suite.labelService.GetLabelRoster(label.ID)

	suite.Require().NoError(err)
	suite.Require().Len(resp, 2)
	// Ordered by name ASC
	suite.Equal("Artist Alpha", resp[0].Name)
	suite.Equal("Artist Beta", resp[1].Name)
}

func (suite *LabelServiceIntegrationTestSuite) TestGetLabelRoster_Empty() {
	label := suite.createTestLabel("Empty Roster Label")

	resp, err := suite.labelService.GetLabelRoster(label.ID)

	suite.Require().NoError(err)
	suite.Empty(resp)
}

func (suite *LabelServiceIntegrationTestSuite) TestGetLabelRoster_LabelNotFound() {
	resp, err := suite.labelService.GetLabelRoster(99999)

	suite.Require().Error(err)
	suite.Nil(resp)
	var labelErr *apperrors.LabelError
	suite.ErrorAs(err, &labelErr)
	suite.Equal(apperrors.CodeLabelNotFound, labelErr.Code)
}

// =============================================================================
// Group 7: GetLabelCatalog
// =============================================================================

func (suite *LabelServiceIntegrationTestSuite) TestGetLabelCatalog_Success() {
	label := suite.createTestLabel("Catalog Label")
	release1 := suite.createTestReleaseForLabel("Release Alpha")
	release2 := suite.createTestReleaseForLabel("Release Beta")

	catNum := "CAT-001"
	suite.db.Exec("INSERT INTO release_labels (release_id, label_id, catalog_number) VALUES (?, ?, ?)", release1.ID, label.ID, catNum)
	suite.db.Exec("INSERT INTO release_labels (release_id, label_id) VALUES (?, ?)", release2.ID, label.ID)

	resp, err := suite.labelService.GetLabelCatalog(label.ID)

	suite.Require().NoError(err)
	suite.Require().Len(resp, 2)

	// Check catalog number is returned for the one that has it
	var withCatalog *contracts.LabelReleaseResponse
	for _, r := range resp {
		if r.CatalogNumber != nil {
			withCatalog = r
		}
	}
	suite.Require().NotNil(withCatalog)
	suite.Equal("CAT-001", *withCatalog.CatalogNumber)
}

func (suite *LabelServiceIntegrationTestSuite) TestGetLabelCatalog_Empty() {
	label := suite.createTestLabel("Empty Catalog Label")

	resp, err := suite.labelService.GetLabelCatalog(label.ID)

	suite.Require().NoError(err)
	suite.Empty(resp)
}

func (suite *LabelServiceIntegrationTestSuite) TestGetLabelCatalog_LabelNotFound() {
	resp, err := suite.labelService.GetLabelCatalog(99999)

	suite.Require().Error(err)
	suite.Nil(resp)
	var labelErr *apperrors.LabelError
	suite.ErrorAs(err, &labelErr)
	suite.Equal(apperrors.CodeLabelNotFound, labelErr.Code)
}

// =============================================================================
// Group 8: DetailResponse counts
// =============================================================================

func (suite *LabelServiceIntegrationTestSuite) TestGetLabel_WithCounts() {
	label := suite.createTestLabel("Counts Label")
	artist1 := suite.createTestArtistForLabel("Count Artist 1")
	artist2 := suite.createTestArtistForLabel("Count Artist 2")
	release1 := suite.createTestReleaseForLabel("Count Release 1")

	suite.db.Exec("INSERT INTO artist_labels (artist_id, label_id) VALUES (?, ?)", artist1.ID, label.ID)
	suite.db.Exec("INSERT INTO artist_labels (artist_id, label_id) VALUES (?, ?)", artist2.ID, label.ID)
	suite.db.Exec("INSERT INTO release_labels (release_id, label_id) VALUES (?, ?)", release1.ID, label.ID)

	resp, err := suite.labelService.GetLabel(label.ID)

	suite.Require().NoError(err)
	suite.Equal(2, resp.ArtistCount)
	suite.Equal(1, resp.ReleaseCount)
}
