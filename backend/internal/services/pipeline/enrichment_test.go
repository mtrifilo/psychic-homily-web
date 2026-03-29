package pipeline

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

func TestEnrichmentService_InvalidEnrichmentType(t *testing.T) {
	// Use a non-nil DB pointer to pass the nil check, but won't actually use it
	svc := &EnrichmentService{db: &gorm.DB{}}
	err := svc.QueueShowForEnrichment(1, "invalid_type")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid enrichment type")
}

func TestEnrichmentService_ValidEnrichmentTypes(t *testing.T) {
	// Verify that the validation switch accepts all valid types.
	// We can't call QueueShowForEnrichment with a zero-value gorm.DB because
	// GORM panics, so we just verify the type constants are defined properly.
	validTypes := []string{
		models.EnrichmentTypeArtistMatch,
		models.EnrichmentTypeMusicBrainz,
		models.EnrichmentTypeAPICrossRef,
		models.EnrichmentTypeAll,
	}
	assert.Equal(t, "artist_match", validTypes[0])
	assert.Equal(t, "musicbrainz", validTypes[1])
	assert.Equal(t, "api_crossref", validTypes[2])
	assert.Equal(t, "all", validTypes[3])
}

func TestMusicBrainzClient_NewClient(t *testing.T) {
	client := NewMusicBrainzClient()
	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
	assert.Equal(t, mbRateLimit, client.rateLimit)
	assert.Equal(t, mbMinScore, client.minScore)
}

func TestSeatGeekClient_NotConfigured(t *testing.T) {
	client := NewSeatGeekClient("")
	assert.False(t, client.IsConfigured())

	result, err := client.SearchEvent("Test Venue", time.Now())
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestSeatGeekClient_Configured(t *testing.T) {
	client := NewSeatGeekClient("test_client_id")
	assert.True(t, client.IsConfigured())
}

func TestEnrichmentWorker_NewWorker(t *testing.T) {
	svc := &EnrichmentService{}
	worker := NewEnrichmentWorker(svc)
	assert.NotNil(t, worker)
	assert.Equal(t, DefaultEnrichmentInterval, worker.interval)
	assert.Equal(t, DefaultEnrichmentBatchSize, worker.batchSize)
}

// =============================================================================
// Mock ArtistService for enrichment tests
// =============================================================================

type mockArtistServiceForEnrichment struct {
	searchArtistsFn func(query string) ([]*contracts.ArtistDetailResponse, error)
}

func (m *mockArtistServiceForEnrichment) CreateArtist(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) GetArtist(artistID uint) (*contracts.ArtistDetailResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) GetArtistByName(name string) (*contracts.ArtistDetailResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) GetArtistBySlug(slug string) (*contracts.ArtistDetailResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) GetArtists(filters map[string]interface{}) ([]*contracts.ArtistDetailResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) GetArtistsWithShowCounts(filters map[string]interface{}) ([]*contracts.ArtistWithShowCountResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) UpdateArtist(artistID uint, updates map[string]interface{}) (*contracts.ArtistDetailResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) DeleteArtist(artistID uint) error { return nil }
func (m *mockArtistServiceForEnrichment) SearchArtists(query string) ([]*contracts.ArtistDetailResponse, error) {
	if m.searchArtistsFn != nil {
		return m.searchArtistsFn(query)
	}
	return []*contracts.ArtistDetailResponse{}, nil
}
func (m *mockArtistServiceForEnrichment) GetShowsForArtist(artistID uint, timezone string, limit int, timeFilter string) ([]*contracts.ArtistShowResponse, int64, error) {
	return nil, 0, nil
}
func (m *mockArtistServiceForEnrichment) GetArtistCities() ([]*contracts.ArtistCityResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) GetLabelsForArtist(artistID uint) ([]*contracts.ArtistLabelResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) AddArtistAlias(artistID uint, alias string) (*contracts.ArtistAliasResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) RemoveArtistAlias(aliasID uint) error { return nil }
func (m *mockArtistServiceForEnrichment) GetArtistAliases(artistID uint) ([]*contracts.ArtistAliasResponse, error) {
	return nil, nil
}
func (m *mockArtistServiceForEnrichment) MergeArtists(canonicalID, mergeFromID uint) (*contracts.MergeArtistResult, error) {
	return nil, nil
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type EnrichmentIntegrationTestSuite struct {
	suite.Suite
	container testcontainers.Container
	db        *gorm.DB
	svc       *EnrichmentService
	ctx       context.Context
}

func (s *EnrichmentIntegrationTestSuite) SetupSuite() {
	s.ctx = context.Background()

	container, err := testcontainers.GenericContainer(s.ctx, testcontainers.GenericContainerRequest{
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
	s.Require().NoError(err)
	s.container = container

	host, _ := container.Host(s.ctx)
	port, _ := container.MappedPort(s.ctx, "5432")

	dsn := fmt.Sprintf("host=%s port=%s user=test_user password=test_password dbname=test_db sslmode=disable", host, port.Port())
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	s.Require().NoError(err)
	s.db = db

	sqlDB, err := db.DB()
	s.Require().NoError(err)

	migrationDir, _ := filepath.Abs("../../../db/migrations")
	testutil.RunAllMigrations(s.T(), sqlDB, migrationDir)

	mockArtist := &mockArtistServiceForEnrichment{}
	s.svc = NewEnrichmentService(db, mockArtist, "")
}

func (s *EnrichmentIntegrationTestSuite) TearDownSuite() {
	if s.container != nil {
		s.container.Terminate(s.ctx)
	}
}

func (s *EnrichmentIntegrationTestSuite) SetupTest() {
	// Clean tables between tests
	s.db.Exec("DELETE FROM enrichment_queue")
	s.db.Exec("DELETE FROM show_artists")
	s.db.Exec("DELETE FROM show_venues")
	s.db.Exec("DELETE FROM shows")
	s.db.Exec("DELETE FROM artists")
	s.db.Exec("DELETE FROM venues")
}

func (s *EnrichmentIntegrationTestSuite) createTestShow() uint {
	show := models.Show{
		Title:     "Test Show",
		EventDate: time.Now().Add(24 * time.Hour),
		Status:    models.ShowStatusApproved,
		Source:    models.ShowSourceDiscovery,
	}
	s.Require().NoError(s.db.Create(&show).Error)
	return show.ID
}

func (s *EnrichmentIntegrationTestSuite) createTestShowWithArtist() (uint, uint) {
	artist := models.Artist{Name: fmt.Sprintf("Test Artist %d-%d", time.Now().UnixNano(), rand.Intn(1000000))}
	s.Require().NoError(s.db.Create(&artist).Error)

	venue := models.Venue{Name: fmt.Sprintf("Test Venue %d-%d", time.Now().UnixNano(), rand.Intn(1000000)), City: "Phoenix", State: "AZ"}
	s.Require().NoError(s.db.Create(&venue).Error)

	show := models.Show{
		Title:     "Test Show with Artist",
		EventDate: time.Now().Add(24 * time.Hour),
		Status:    models.ShowStatusApproved,
		Source:    models.ShowSourceDiscovery,
	}
	s.Require().NoError(s.db.Create(&show).Error)

	showArtist := models.ShowArtist{ShowID: show.ID, ArtistID: artist.ID, SetType: "headliner"}
	s.Require().NoError(s.db.Create(&showArtist).Error)

	showVenue := models.ShowVenue{ShowID: show.ID, VenueID: venue.ID}
	s.Require().NoError(s.db.Create(&showVenue).Error)

	return show.ID, artist.ID
}

// Test: QueueShowForEnrichment
func (s *EnrichmentIntegrationTestSuite) TestQueueShowForEnrichment() {
	showID := s.createTestShow()

	err := s.svc.QueueShowForEnrichment(showID, models.EnrichmentTypeAll)
	s.Require().NoError(err)

	// Verify item was created
	var item models.EnrichmentQueueItem
	err = s.db.Where("show_id = ?", showID).First(&item).Error
	s.Require().NoError(err)
	s.Equal(models.EnrichmentStatusPending, item.Status)
	s.Equal(models.EnrichmentTypeAll, item.EnrichmentType)
	s.Equal(0, item.Attempts)
	s.Equal(3, item.MaxAttempts)
}

// Test: ProcessQueue - empty queue
func (s *EnrichmentIntegrationTestSuite) TestProcessQueue_EmptyQueue() {
	processed, err := s.svc.ProcessQueue(s.ctx, 10)
	s.Require().NoError(err)
	s.Equal(0, processed)
}

// Test: ProcessQueue - processes pending items
func (s *EnrichmentIntegrationTestSuite) TestProcessQueue_ProcessesPending() {
	showID, _ := s.createTestShowWithArtist()

	// Queue the show
	err := s.svc.QueueShowForEnrichment(showID, models.EnrichmentTypeAll)
	s.Require().NoError(err)

	// Process the queue
	processed, err := s.svc.ProcessQueue(s.ctx, 10)
	s.Require().NoError(err)
	s.Equal(1, processed)

	// Verify item was completed
	var item models.EnrichmentQueueItem
	err = s.db.Where("show_id = ?", showID).First(&item).Error
	s.Require().NoError(err)
	s.Equal(models.EnrichmentStatusCompleted, item.Status)
	s.Equal(1, item.Attempts)
	s.NotNil(item.CompletedAt)
	s.NotNil(item.Results)
}

// Test: ProcessQueue - respects batch size
func (s *EnrichmentIntegrationTestSuite) TestProcessQueue_RespectsBatchSize() {
	// Create 3 shows and queue them
	for i := 0; i < 3; i++ {
		showID, _ := s.createTestShowWithArtist()
		s.Require().NoError(s.svc.QueueShowForEnrichment(showID, models.EnrichmentTypeAll))
	}

	// Process only 2
	processed, err := s.svc.ProcessQueue(s.ctx, 2)
	s.Require().NoError(err)
	s.Equal(2, processed)

	// Verify 1 still pending
	var pendingCount int64
	s.db.Model(&models.EnrichmentQueueItem{}).
		Where("status = ?", models.EnrichmentStatusPending).
		Count(&pendingCount)
	s.Equal(int64(1), pendingCount)
}

// Test: ProcessQueue - skips items at max attempts
func (s *EnrichmentIntegrationTestSuite) TestProcessQueue_SkipsMaxAttempts() {
	showID := s.createTestShow()

	// Create a queue item already at max attempts
	item := &models.EnrichmentQueueItem{
		ShowID:         showID,
		Status:         models.EnrichmentStatusPending,
		Attempts:       3,
		MaxAttempts:    3,
		EnrichmentType: models.EnrichmentTypeAll,
	}
	s.Require().NoError(s.db.Create(item).Error)

	// Process — should not pick up this item
	processed, err := s.svc.ProcessQueue(s.ctx, 10)
	s.Require().NoError(err)
	s.Equal(0, processed)
}

// Test: EnrichShow - show not found
func (s *EnrichmentIntegrationTestSuite) TestEnrichShow_ShowNotFound() {
	_, err := s.svc.EnrichShow(s.ctx, 999999)
	s.Error(err)
	s.Contains(err.Error(), "show not found")
}

// Test: EnrichShow - successful enrichment
func (s *EnrichmentIntegrationTestSuite) TestEnrichShow_Success() {
	showID, _ := s.createTestShowWithArtist()

	result, err := s.svc.EnrichShow(s.ctx, showID)
	s.Require().NoError(err)
	s.NotNil(result)
	s.Equal(showID, result.ShowID)
	s.Contains(result.CompletedSteps, "artist_match")
	s.Contains(result.CompletedSteps, "musicbrainz")
	s.Contains(result.CompletedSteps, "api_crossref")
}

// Test: EnrichShow - context cancellation
func (s *EnrichmentIntegrationTestSuite) TestEnrichShow_ContextCancellation() {
	showID, _ := s.createTestShowWithArtist()

	ctx, cancel := context.WithCancel(s.ctx)
	cancel() // Cancel immediately

	result, err := s.svc.EnrichShow(ctx, showID)
	// Should still return partial result (at least artist_match step)
	s.NoError(err)
	s.NotNil(result)
	s.Equal(showID, result.ShowID)
}

// Test: GetQueueStats
func (s *EnrichmentIntegrationTestSuite) TestGetQueueStats() {
	// Create some items in different states
	showID1 := s.createTestShow()
	showID2 := s.createTestShow()
	showID3 := s.createTestShow()

	s.db.Create(&models.EnrichmentQueueItem{
		ShowID: showID1, Status: models.EnrichmentStatusPending, EnrichmentType: models.EnrichmentTypeAll,
	})
	s.db.Create(&models.EnrichmentQueueItem{
		ShowID: showID2, Status: models.EnrichmentStatusProcessing, EnrichmentType: models.EnrichmentTypeAll,
	})
	now := time.Now()
	s.db.Create(&models.EnrichmentQueueItem{
		ShowID: showID3, Status: models.EnrichmentStatusCompleted, EnrichmentType: models.EnrichmentTypeAll,
		CompletedAt: &now,
	})

	stats, err := s.svc.GetQueueStats()
	s.Require().NoError(err)
	s.Equal(int64(1), stats.Pending)
	s.Equal(int64(1), stats.Processing)
	s.Equal(int64(1), stats.CompletedToday)
}

// Test: SeatGeek enrichment skipped when not configured
func (s *EnrichmentIntegrationTestSuite) TestEnrichShow_SeatGeekSkippedWhenNotConfigured() {
	showID, _ := s.createTestShowWithArtist()

	result, err := s.svc.EnrichShow(s.ctx, showID)
	s.Require().NoError(err)
	s.NotNil(result.SeatGeek)
	s.False(result.SeatGeek.Found) // SeatGeek not configured, so no results
}

func TestEnrichmentIntegrationTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration tests in short mode")
	}
	suite.Run(t, new(EnrichmentIntegrationTestSuite))
}
