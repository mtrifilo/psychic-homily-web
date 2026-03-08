package services

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewReleaseService(t *testing.T) {
	releaseService := NewReleaseService(nil)
	assert.NotNil(t, releaseService)
}

func TestReleaseService_NilDatabase(t *testing.T) {
	svc := &ReleaseService{db: nil}

	t.Run("CreateRelease", func(t *testing.T) {
		resp, err := svc.CreateRelease(&CreateReleaseRequest{Title: "Test"})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetRelease", func(t *testing.T) {
		resp, err := svc.GetRelease(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetReleaseBySlug", func(t *testing.T) {
		resp, err := svc.GetReleaseBySlug("test-slug")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("ListReleases", func(t *testing.T) {
		resp, err := svc.ListReleases(nil)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("UpdateRelease", func(t *testing.T) {
		resp, err := svc.UpdateRelease(1, &UpdateReleaseRequest{})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("DeleteRelease", func(t *testing.T) {
		err := svc.DeleteRelease(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("GetReleasesForArtist", func(t *testing.T) {
		resp, err := svc.GetReleasesForArtist(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("AddExternalLink", func(t *testing.T) {
		resp, err := svc.AddExternalLink(1, "bandcamp", "http://test.com")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("RemoveExternalLink", func(t *testing.T) {
		err := svc.RemoveExternalLink(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type ReleaseServiceIntegrationTestSuite struct {
	suite.Suite
	container      testcontainers.Container
	db             *gorm.DB
	releaseService *ReleaseService
	artistService  *ArtistService
	ctx            context.Context
}

func (suite *ReleaseServiceIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	// Start PostgreSQL container
	container, err := testcontainers.GenericContainer(suite.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:18",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_DB":       "test_db",
				"POSTGRES_USER":     "test_user",
				"POSTGRES_PASSWORD": "test_password",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		suite.T().Fatalf("failed to start postgres container: %v", err)
	}
	suite.container = container

	host, err := container.Host(suite.ctx)
	if err != nil {
		suite.T().Fatalf("failed to get host: %v", err)
	}
	port, err := container.MappedPort(suite.ctx, "5432")
	if err != nil {
		suite.T().Fatalf("failed to get port: %v", err)
	}

	dsn := fmt.Sprintf("host=%s port=%s user=test_user password=test_password dbname=test_db sslmode=disable",
		host, port.Port())

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		suite.T().Fatalf("failed to connect to test database: %v", err)
	}
	suite.db = db

	sqlDB, err := db.DB()
	if err != nil {
		suite.T().Fatalf("failed to get sql.DB: %v", err)
	}
	testutil.RunAllMigrations(suite.T(), sqlDB, filepath.Join("..", "..", "db", "migrations"))

	suite.releaseService = &ReleaseService{db: db}
	suite.artistService = &ArtistService{db: db}
}

func (suite *ReleaseServiceIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
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

func intPtr(i int) *int {
	return &i
}

// =============================================================================
// Group 1: CreateRelease
// =============================================================================

func (suite *ReleaseServiceIntegrationTestSuite) TestCreateRelease_Success() {
	year := 2024
	req := &CreateReleaseRequest{
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
	req := &CreateReleaseRequest{
		Title:       "Nevermind",
		ReleaseType: "lp",
		ReleaseYear: &year,
		Artists: []CreateReleaseArtistEntry{
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
	req := &CreateReleaseRequest{
		Title:       "In Utero",
		ReleaseType: "lp",
		ExternalLinks: []CreateReleaseLinkEntry{
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
	req := &CreateReleaseRequest{
		Title: "Mystery Release",
	}

	resp, err := suite.releaseService.CreateRelease(req)

	suite.Require().NoError(err)
	suite.Equal("lp", resp.ReleaseType)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestCreateRelease_UniqueSlug() {
	req1 := &CreateReleaseRequest{Title: "Homesick"}
	resp1, err := suite.releaseService.CreateRelease(req1)
	suite.Require().NoError(err)

	req2 := &CreateReleaseRequest{Title: "Homesick"}
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
	created, err := suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "Test Album"})
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
	created, err := suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "Slug Test Album"})
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
	suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "Album A", ReleaseYear: intPtr(2020)})
	suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "Album B", ReleaseYear: intPtr(2023)})
	suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "Album C", ReleaseYear: intPtr(2021)})

	resp, err := suite.releaseService.ListReleases(map[string]interface{}{})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 3)
	// Ordered by release_year DESC, title ASC
	suite.Equal("Album B", resp[0].Title) // 2023
	suite.Equal("Album C", resp[1].Title) // 2021
	suite.Equal("Album A", resp[2].Title) // 2020
}

func (suite *ReleaseServiceIntegrationTestSuite) TestListReleases_FilterByType() {
	suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "LP Release", ReleaseType: "lp"})
	suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "EP Release", ReleaseType: "ep"})
	suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "Single Release", ReleaseType: "single"})

	resp, err := suite.releaseService.ListReleases(map[string]interface{}{"release_type": "ep"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("EP Release", resp[0].Title)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestListReleases_FilterByYear() {
	suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "Old Album", ReleaseYear: intPtr(2020)})
	suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "New Album", ReleaseYear: intPtr(2024)})

	resp, err := suite.releaseService.ListReleases(map[string]interface{}{"year": 2024})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("New Album", resp[0].Title)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestListReleases_FilterByArtist() {
	artist1 := suite.createTestArtist("Artist One")
	artist2 := suite.createTestArtist("Artist Two")

	suite.releaseService.CreateRelease(&CreateReleaseRequest{
		Title:   "Artist One Album",
		Artists: []CreateReleaseArtistEntry{{ArtistID: artist1.ID, Role: "main"}},
	})
	suite.releaseService.CreateRelease(&CreateReleaseRequest{
		Title:   "Artist Two Album",
		Artists: []CreateReleaseArtistEntry{{ArtistID: artist2.ID, Role: "main"}},
	})

	resp, err := suite.releaseService.ListReleases(map[string]interface{}{"artist_id": artist1.ID})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Artist One Album", resp[0].Title)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestListReleases_ArtistCount() {
	artist1 := suite.createTestArtist("Count Artist 1")
	artist2 := suite.createTestArtist("Count Artist 2")

	suite.releaseService.CreateRelease(&CreateReleaseRequest{
		Title: "Multi Artist Album",
		Artists: []CreateReleaseArtistEntry{
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
	created, err := suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "Original Title", ReleaseYear: intPtr(2020)})
	suite.Require().NoError(err)

	newTitle := "Updated Title"
	newYear := 2021
	resp, err := suite.releaseService.UpdateRelease(created.ID, &UpdateReleaseRequest{
		Title:       &newTitle,
		ReleaseYear: &newYear,
	})

	suite.Require().NoError(err)
	suite.Equal("Updated Title", resp.Title)
	suite.Equal(2021, *resp.ReleaseYear)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestUpdateRelease_TitleChangeRegeneratesSlug() {
	created, err := suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "Old Title"})
	suite.Require().NoError(err)
	oldSlug := created.Slug

	newTitle := "New Title"
	resp, err := suite.releaseService.UpdateRelease(created.ID, &UpdateReleaseRequest{
		Title: &newTitle,
	})

	suite.Require().NoError(err)
	suite.Equal("New Title", resp.Title)
	suite.NotEqual(oldSlug, resp.Slug)
	suite.Equal("new-title", resp.Slug)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestUpdateRelease_NotFound() {
	newTitle := "Anything"
	resp, err := suite.releaseService.UpdateRelease(99999, &UpdateReleaseRequest{Title: &newTitle})

	suite.Require().Error(err)
	suite.Nil(resp)
	var releaseErr *apperrors.ReleaseError
	suite.ErrorAs(err, &releaseErr)
	suite.Equal(apperrors.CodeReleaseNotFound, releaseErr.Code)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestUpdateRelease_NoChanges() {
	created, err := suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "Stable Title"})
	suite.Require().NoError(err)

	// Update with no fields set
	resp, err := suite.releaseService.UpdateRelease(created.ID, &UpdateReleaseRequest{})

	suite.Require().NoError(err)
	suite.Equal("Stable Title", resp.Title)
}

// =============================================================================
// Group 5: DeleteRelease
// =============================================================================

func (suite *ReleaseServiceIntegrationTestSuite) TestDeleteRelease_Success() {
	created, err := suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "Delete Me"})
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
	created, err := suite.releaseService.CreateRelease(&CreateReleaseRequest{
		Title:   "Cascade Album",
		Artists: []CreateReleaseArtistEntry{{ArtistID: artist.ID, Role: "main"}},
		ExternalLinks: []CreateReleaseLinkEntry{
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

	suite.releaseService.CreateRelease(&CreateReleaseRequest{
		Title:       "Album A",
		ReleaseYear: intPtr(2020),
		Artists:     []CreateReleaseArtistEntry{{ArtistID: artist.ID, Role: "main"}},
	})
	suite.releaseService.CreateRelease(&CreateReleaseRequest{
		Title:       "Album B",
		ReleaseYear: intPtr(2023),
		Artists:     []CreateReleaseArtistEntry{{ArtistID: artist.ID, Role: "main"}},
	})
	suite.releaseService.CreateRelease(&CreateReleaseRequest{
		Title:   "Other Album",
		Artists: []CreateReleaseArtistEntry{{ArtistID: otherArtist.ID, Role: "main"}},
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
	created, err := suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "Link Album"})
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
	created, err := suite.releaseService.CreateRelease(&CreateReleaseRequest{
		Title: "Remove Link Album",
		ExternalLinks: []CreateReleaseLinkEntry{
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
	created, err := suite.releaseService.CreateRelease(&CreateReleaseRequest{Title: "Multi Link Album"})
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
