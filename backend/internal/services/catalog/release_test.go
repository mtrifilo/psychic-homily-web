package catalog

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type ReleaseServiceIntegrationTestSuite struct {
	suite.Suite
	testDB         *testutil.TestDatabase
	db             *gorm.DB
	releaseService *ReleaseService
	artistService  *ArtistService
}

func (suite *ReleaseServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.releaseService = &ReleaseService{db: suite.testDB.DB}
	suite.artistService = &ArtistService{db: suite.testDB.DB}
}

func (suite *ReleaseServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

// TearDownTest cleans up data between tests for isolation
func (suite *ReleaseServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM release_external_links")
	_, _ = sqlDB.Exec("DELETE FROM artist_releases")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

func TestReleaseServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ReleaseServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *ReleaseServiceIntegrationTestSuite) createTestArtist(name string) *models.Artist {
	artist := &models.Artist{
		Name: name,
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

// =============================================================================
// Group 1: CreateRelease
// =============================================================================

func (suite *ReleaseServiceIntegrationTestSuite) TestCreateRelease_Success() {
	year := 2024
	req := &contracts.CreateReleaseRequest{
		Title:       "Nevermind",
		ReleaseType: "lp",
		ReleaseYear: &year,
	}

	resp, err := suite.releaseService.CreateRelease(req)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal("Nevermind", resp.Title)
	suite.Equal("nevermind", resp.Slug)
	suite.Equal("lp", resp.ReleaseType)
	suite.Equal(2024, *resp.ReleaseYear)
	suite.Empty(resp.Artists)
	suite.Empty(resp.ExternalLinks)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestCreateRelease_WithArtists() {
	artist1 := suite.createTestArtist("Nirvana")
	artist2 := suite.createTestArtist("Butch Vig")

	year := 1991
	req := &contracts.CreateReleaseRequest{
		Title:       "Nevermind",
		ReleaseType: "lp",
		ReleaseYear: &year,
		Artists: []contracts.CreateReleaseArtistEntry{
			{ArtistID: artist1.ID, Role: "main"},
			{ArtistID: artist2.ID, Role: "producer"},
		},
	}

	resp, err := suite.releaseService.CreateRelease(req)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Artists, 2)
	suite.Equal("Nirvana", resp.Artists[0].Name)
	suite.Equal("main", resp.Artists[0].Role)
	suite.Equal("Butch Vig", resp.Artists[1].Name)
	suite.Equal("producer", resp.Artists[1].Role)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestCreateRelease_WithExternalLinks() {
	req := &contracts.CreateReleaseRequest{
		Title:       "In Utero",
		ReleaseType: "lp",
		ExternalLinks: []contracts.CreateReleaseLinkEntry{
			{Platform: "bandcamp", URL: "https://nirvana.bandcamp.com/album/in-utero"},
			{Platform: "spotify", URL: "https://open.spotify.com/album/abc123"},
		},
	}

	resp, err := suite.releaseService.CreateRelease(req)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.ExternalLinks, 2)
	suite.Equal("bandcamp", resp.ExternalLinks[0].Platform)
	suite.Equal("spotify", resp.ExternalLinks[1].Platform)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestCreateRelease_DefaultReleaseType() {
	req := &contracts.CreateReleaseRequest{
		Title: "Mystery Release",
	}

	resp, err := suite.releaseService.CreateRelease(req)

	suite.Require().NoError(err)
	suite.Equal("lp", resp.ReleaseType)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestCreateRelease_UniqueSlug() {
	req1 := &contracts.CreateReleaseRequest{Title: "Homesick"}
	resp1, err := suite.releaseService.CreateRelease(req1)
	suite.Require().NoError(err)

	req2 := &contracts.CreateReleaseRequest{Title: "Homesick"}
	resp2, err := suite.releaseService.CreateRelease(req2)
	suite.Require().NoError(err)

	suite.NotEqual(resp1.Slug, resp2.Slug)
	suite.Equal("homesick", resp1.Slug)
	suite.Equal("homesick-2", resp2.Slug)
}

// =============================================================================
// Group 2: GetRelease / GetReleaseBySlug
// =============================================================================

func (suite *ReleaseServiceIntegrationTestSuite) TestGetRelease_Success() {
	created, err := suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Test Album"})
	suite.Require().NoError(err)

	resp, err := suite.releaseService.GetRelease(created.ID)

	suite.Require().NoError(err)
	suite.Equal(created.ID, resp.ID)
	suite.Equal("Test Album", resp.Title)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestGetRelease_NotFound() {
	resp, err := suite.releaseService.GetRelease(99999)

	suite.Require().Error(err)
	suite.Nil(resp)
	var releaseErr *apperrors.ReleaseError
	suite.ErrorAs(err, &releaseErr)
	suite.Equal(apperrors.CodeReleaseNotFound, releaseErr.Code)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestGetReleaseBySlug_Success() {
	created, err := suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Slug Test Album"})
	suite.Require().NoError(err)

	resp, err := suite.releaseService.GetReleaseBySlug(created.Slug)

	suite.Require().NoError(err)
	suite.Equal(created.ID, resp.ID)
	suite.Equal(created.Slug, resp.Slug)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestGetReleaseBySlug_NotFound() {
	resp, err := suite.releaseService.GetReleaseBySlug("nonexistent-slug-xyz")

	suite.Require().Error(err)
	suite.Nil(resp)
	var releaseErr *apperrors.ReleaseError
	suite.ErrorAs(err, &releaseErr)
	suite.Equal(apperrors.CodeReleaseNotFound, releaseErr.Code)
}

// =============================================================================
// Group 3: ListReleases
// =============================================================================

func (suite *ReleaseServiceIntegrationTestSuite) TestListReleases_All() {
	suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Album A", ReleaseYear: intPtr(2020)})
	suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Album B", ReleaseYear: intPtr(2023)})
	suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Album C", ReleaseYear: intPtr(2021)})

	resp, err := suite.releaseService.ListReleases(map[string]interface{}{})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 3)
	// Ordered by release_year DESC, title ASC
	suite.Equal("Album B", resp[0].Title) // 2023
	suite.Equal("Album C", resp[1].Title) // 2021
	suite.Equal("Album A", resp[2].Title) // 2020
}

func (suite *ReleaseServiceIntegrationTestSuite) TestListReleases_FilterByType() {
	suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "LP Release", ReleaseType: "lp"})
	suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "EP Release", ReleaseType: "ep"})
	suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Single Release", ReleaseType: "single"})

	resp, err := suite.releaseService.ListReleases(map[string]interface{}{"release_type": "ep"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("EP Release", resp[0].Title)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestListReleases_FilterByYear() {
	suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Old Album", ReleaseYear: intPtr(2020)})
	suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "New Album", ReleaseYear: intPtr(2024)})

	resp, err := suite.releaseService.ListReleases(map[string]interface{}{"year": 2024})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("New Album", resp[0].Title)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestListReleases_FilterByArtist() {
	artist1 := suite.createTestArtist("Artist One")
	artist2 := suite.createTestArtist("Artist Two")

	suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{
		Title:   "Artist One Album",
		Artists: []contracts.CreateReleaseArtistEntry{{ArtistID: artist1.ID, Role: "main"}},
	})
	suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{
		Title:   "Artist Two Album",
		Artists: []contracts.CreateReleaseArtistEntry{{ArtistID: artist2.ID, Role: "main"}},
	})

	resp, err := suite.releaseService.ListReleases(map[string]interface{}{"artist_id": artist1.ID})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Artist One Album", resp[0].Title)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestListReleases_ArtistCount() {
	artist1 := suite.createTestArtist("Count Artist 1")
	artist2 := suite.createTestArtist("Count Artist 2")

	suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{
		Title: "Multi Artist Album",
		Artists: []contracts.CreateReleaseArtistEntry{
			{ArtistID: artist1.ID, Role: "main"},
			{ArtistID: artist2.ID, Role: "featured"},
		},
	})

	resp, err := suite.releaseService.ListReleases(map[string]interface{}{})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal(2, resp[0].ArtistCount)
}

// =============================================================================
// Group 4: UpdateRelease
// =============================================================================

func (suite *ReleaseServiceIntegrationTestSuite) TestUpdateRelease_BasicFields() {
	created, err := suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Original Title", ReleaseYear: intPtr(2020)})
	suite.Require().NoError(err)

	newTitle := "Updated Title"
	newYear := 2021
	resp, err := suite.releaseService.UpdateRelease(created.ID, &contracts.UpdateReleaseRequest{
		Title:       &newTitle,
		ReleaseYear: &newYear,
	})

	suite.Require().NoError(err)
	suite.Equal("Updated Title", resp.Title)
	suite.Equal(2021, *resp.ReleaseYear)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestUpdateRelease_TitleChangeRegeneratesSlug() {
	created, err := suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Old Title"})
	suite.Require().NoError(err)
	oldSlug := created.Slug

	newTitle := "New Title"
	resp, err := suite.releaseService.UpdateRelease(created.ID, &contracts.UpdateReleaseRequest{
		Title: &newTitle,
	})

	suite.Require().NoError(err)
	suite.Equal("New Title", resp.Title)
	suite.NotEqual(oldSlug, resp.Slug)
	suite.Equal("new-title", resp.Slug)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestUpdateRelease_NotFound() {
	newTitle := "Anything"
	resp, err := suite.releaseService.UpdateRelease(99999, &contracts.UpdateReleaseRequest{Title: &newTitle})

	suite.Require().Error(err)
	suite.Nil(resp)
	var releaseErr *apperrors.ReleaseError
	suite.ErrorAs(err, &releaseErr)
	suite.Equal(apperrors.CodeReleaseNotFound, releaseErr.Code)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestUpdateRelease_NoChanges() {
	created, err := suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Stable Title"})
	suite.Require().NoError(err)

	// Update with no fields set
	resp, err := suite.releaseService.UpdateRelease(created.ID, &contracts.UpdateReleaseRequest{})

	suite.Require().NoError(err)
	suite.Equal("Stable Title", resp.Title)
}

// =============================================================================
// Group 5: DeleteRelease
// =============================================================================

func (suite *ReleaseServiceIntegrationTestSuite) TestDeleteRelease_Success() {
	created, err := suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Delete Me"})
	suite.Require().NoError(err)

	err = suite.releaseService.DeleteRelease(created.ID)

	suite.Require().NoError(err)

	// Verify it's gone
	_, err = suite.releaseService.GetRelease(created.ID)
	suite.Error(err)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestDeleteRelease_NotFound() {
	err := suite.releaseService.DeleteRelease(99999)

	suite.Require().Error(err)
	var releaseErr *apperrors.ReleaseError
	suite.ErrorAs(err, &releaseErr)
	suite.Equal(apperrors.CodeReleaseNotFound, releaseErr.Code)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestDeleteRelease_CascadesJunctions() {
	artist := suite.createTestArtist("Cascade Artist")
	created, err := suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{
		Title:   "Cascade Album",
		Artists: []contracts.CreateReleaseArtistEntry{{ArtistID: artist.ID, Role: "main"}},
		ExternalLinks: []contracts.CreateReleaseLinkEntry{
			{Platform: "bandcamp", URL: "https://test.bandcamp.com"},
		},
	})
	suite.Require().NoError(err)

	err = suite.releaseService.DeleteRelease(created.ID)
	suite.Require().NoError(err)

	// Verify artist_releases cleaned up
	var arCount int64
	suite.db.Model(&models.ArtistRelease{}).Where("release_id = ?", created.ID).Count(&arCount)
	suite.Equal(int64(0), arCount)

	// Verify external_links cleaned up
	var linkCount int64
	suite.db.Model(&models.ReleaseExternalLink{}).Where("release_id = ?", created.ID).Count(&linkCount)
	suite.Equal(int64(0), linkCount)
}

// =============================================================================
// Group 6: GetReleasesForArtist
// =============================================================================

func (suite *ReleaseServiceIntegrationTestSuite) TestGetReleasesForArtist_Success() {
	artist := suite.createTestArtist("Discography Artist")
	otherArtist := suite.createTestArtist("Other Artist")

	suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{
		Title:       "Album A",
		ReleaseYear: intPtr(2020),
		Artists:     []contracts.CreateReleaseArtistEntry{{ArtistID: artist.ID, Role: "main"}},
	})
	suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{
		Title:       "Album B",
		ReleaseYear: intPtr(2023),
		Artists:     []contracts.CreateReleaseArtistEntry{{ArtistID: artist.ID, Role: "main"}},
	})
	suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{
		Title:   "Other Album",
		Artists: []contracts.CreateReleaseArtistEntry{{ArtistID: otherArtist.ID, Role: "main"}},
	})

	resp, err := suite.releaseService.GetReleasesForArtist(artist.ID)

	suite.Require().NoError(err)
	suite.Require().Len(resp, 2)
	// Ordered by year DESC
	suite.Equal("Album B", resp[0].Title)
	suite.Equal("Album A", resp[1].Title)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestGetReleasesForArtist_ArtistNotFound() {
	resp, err := suite.releaseService.GetReleasesForArtist(99999)

	suite.Require().Error(err)
	suite.Nil(resp)
	var artistErr *apperrors.ArtistError
	suite.ErrorAs(err, &artistErr)
	suite.Equal(apperrors.CodeArtistNotFound, artistErr.Code)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestGetReleasesForArtist_Empty() {
	artist := suite.createTestArtist("No Releases Artist")

	resp, err := suite.releaseService.GetReleasesForArtist(artist.ID)

	suite.Require().NoError(err)
	suite.Empty(resp)
}

// =============================================================================
// Group 7: ExternalLinks
// =============================================================================

func (suite *ReleaseServiceIntegrationTestSuite) TestAddExternalLink_Success() {
	created, err := suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Link Album"})
	suite.Require().NoError(err)

	link, err := suite.releaseService.AddExternalLink(created.ID, "bandcamp", "https://test.bandcamp.com/album/test")

	suite.Require().NoError(err)
	suite.Require().NotNil(link)
	suite.NotZero(link.ID)
	suite.Equal("bandcamp", link.Platform)
	suite.Equal("https://test.bandcamp.com/album/test", link.URL)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestAddExternalLink_ReleaseNotFound() {
	link, err := suite.releaseService.AddExternalLink(99999, "bandcamp", "https://test.bandcamp.com")

	suite.Require().Error(err)
	suite.Nil(link)
	var releaseErr *apperrors.ReleaseError
	suite.ErrorAs(err, &releaseErr)
	suite.Equal(apperrors.CodeReleaseNotFound, releaseErr.Code)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestRemoveExternalLink_Success() {
	created, err := suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{
		Title: "Remove Link Album",
		ExternalLinks: []contracts.CreateReleaseLinkEntry{
			{Platform: "spotify", URL: "https://open.spotify.com/album/abc"},
		},
	})
	suite.Require().NoError(err)
	suite.Require().Len(created.ExternalLinks, 1)

	err = suite.releaseService.RemoveExternalLink(created.ExternalLinks[0].ID)

	suite.Require().NoError(err)

	// Verify it's gone
	refreshed, err := suite.releaseService.GetRelease(created.ID)
	suite.Require().NoError(err)
	suite.Empty(refreshed.ExternalLinks)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestRemoveExternalLink_NotFound() {
	err := suite.releaseService.RemoveExternalLink(99999)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "external link not found")
}

func (suite *ReleaseServiceIntegrationTestSuite) TestAddMultipleExternalLinks() {
	created, err := suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Multi Link Album"})
	suite.Require().NoError(err)

	_, err = suite.releaseService.AddExternalLink(created.ID, "bandcamp", "https://test.bandcamp.com")
	suite.Require().NoError(err)
	_, err = suite.releaseService.AddExternalLink(created.ID, "spotify", "https://open.spotify.com/album/test")
	suite.Require().NoError(err)
	_, err = suite.releaseService.AddExternalLink(created.ID, "discogs", "https://www.discogs.com/release/123")
	suite.Require().NoError(err)

	refreshed, err := suite.releaseService.GetRelease(created.ID)
	suite.Require().NoError(err)
	suite.Require().Len(refreshed.ExternalLinks, 3)
}
