package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewVenueSourceConfigService(t *testing.T) {
	svc := NewVenueSourceConfigService(nil)
	assert.NotNil(t, svc)
}

func TestVenueSourceConfigService_NilDatabase(t *testing.T) {
	svc := &VenueSourceConfigService{db: nil}

	t.Run("GetByVenueID", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetByVenueID(1)
		})
	})

	t.Run("CreateOrUpdate", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.CreateOrUpdate(&models.VenueSourceConfig{VenueID: 1})
		})
	})

	t.Run("UpdateAfterRun", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			return svc.UpdateAfterRun(1, nil, nil, 5)
		})
	})

	t.Run("IncrementFailures", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			return svc.IncrementFailures(1)
		})
	})

	t.Run("RecordRun", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			return svc.RecordRun(&models.VenueExtractionRun{VenueID: 1})
		})
	})

	t.Run("GetRecentRuns", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetRecentRuns(1, 10)
		})
	})

	t.Run("ListConfigured", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.ListConfigured()
		})
	})

	t.Run("ResetRenderMethod", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			return svc.ResetRenderMethod(1)
		})
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type VenueSourceConfigIntegrationTestSuite struct {
	suite.Suite
	testDB  *testutil.TestDatabase
	db      *gorm.DB
	svc     *VenueSourceConfigService
	venueID uint
}

func (s *VenueSourceConfigIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB

	s.svc = NewVenueSourceConfigService(s.testDB.DB)
}

func (s *VenueSourceConfigIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *VenueSourceConfigIntegrationTestSuite) SetupTest() {
	// Create a test venue
	slug := "test-venue"
	venue := models.Venue{
		Name:  "Test Venue",
		Slug:  &slug,
		City:  "Phoenix",
		State: "AZ",
	}
	s.Require().NoError(s.db.Create(&venue).Error)
	s.venueID = venue.ID
}

func (s *VenueSourceConfigIntegrationTestSuite) TearDownTest() {
	s.db.Exec("DELETE FROM venue_extraction_runs")
	s.db.Exec("DELETE FROM venue_source_configs")
	s.db.Exec("DELETE FROM venues")
}

func TestVenueSourceConfigIntegrationSuite(t *testing.T) {
	suite.Run(t, new(VenueSourceConfigIntegrationTestSuite))
}

// --- CreateOrUpdate tests ---

func (s *VenueSourceConfigIntegrationTestSuite) TestCreateOrUpdate_NewConfig() {
	url := "https://testvenue.com/calendar"
	config := &models.VenueSourceConfig{
		VenueID:     s.venueID,
		CalendarURL: &url,
	}

	result, err := s.svc.CreateOrUpdate(config)
	s.NoError(err)
	s.NotNil(result)
	s.Equal(s.venueID, result.VenueID)
	s.Equal(&url, result.CalendarURL)
	s.Equal("ai", result.PreferredSource)
	s.False(result.AutoApprove) // default is false — requires explicit opt-in
	s.False(result.StrategyLocked)
}

func (s *VenueSourceConfigIntegrationTestSuite) TestCreateOrUpdate_UpdateExisting() {
	url1 := "https://testvenue.com/old"
	config := &models.VenueSourceConfig{
		VenueID:     s.venueID,
		CalendarURL: &url1,
	}
	_, err := s.svc.CreateOrUpdate(config)
	s.NoError(err)

	url2 := "https://testvenue.com/new"
	updated := &models.VenueSourceConfig{
		VenueID:         s.venueID,
		CalendarURL:     &url2,
		PreferredSource: "ical",
	}
	result, err := s.svc.CreateOrUpdate(updated)
	s.NoError(err)
	s.NotNil(result)
	s.Equal(&url2, result.CalendarURL)
	s.Equal("ical", result.PreferredSource)
}

func (s *VenueSourceConfigIntegrationTestSuite) TestCreateOrUpdate_VenueIDRequired() {
	config := &models.VenueSourceConfig{}
	result, err := s.svc.CreateOrUpdate(config)
	s.Error(err)
	s.Contains(err.Error(), "venue_id is required")
	s.Nil(result)
}

// --- GetByVenueID tests ---

func (s *VenueSourceConfigIntegrationTestSuite) TestGetByVenueID_Found() {
	url := "https://testvenue.com/calendar"
	config := &models.VenueSourceConfig{
		VenueID:     s.venueID,
		CalendarURL: &url,
	}
	_, err := s.svc.CreateOrUpdate(config)
	s.NoError(err)

	result, err := s.svc.GetByVenueID(s.venueID)
	s.NoError(err)
	s.NotNil(result)
	s.Equal(s.venueID, result.VenueID)
}

func (s *VenueSourceConfigIntegrationTestSuite) TestGetByVenueID_NotFound() {
	result, err := s.svc.GetByVenueID(99999)
	s.NoError(err)
	s.Nil(result)
}

// --- UpdateAfterRun tests ---

func (s *VenueSourceConfigIntegrationTestSuite) TestUpdateAfterRun_Success() {
	url := "https://testvenue.com/calendar"
	config := &models.VenueSourceConfig{
		VenueID:     s.venueID,
		CalendarURL: &url,
	}
	_, err := s.svc.CreateOrUpdate(config)
	s.NoError(err)

	hash := "abc123"
	etag := "\"etag-1\""
	err = s.svc.UpdateAfterRun(s.venueID, &hash, &etag, 10)
	s.NoError(err)

	result, err := s.svc.GetByVenueID(s.venueID)
	s.NoError(err)
	s.NotNil(result.LastExtractedAt)
	s.Equal(&hash, result.LastContentHash)
	s.Equal(&etag, result.LastETag)
	s.Equal(0, result.ConsecutiveFailures)
	s.Equal(5, result.EventsExpected) // (0 + 10) / 2
}

func (s *VenueSourceConfigIntegrationTestSuite) TestUpdateAfterRun_RollingAverage() {
	url := "https://testvenue.com/calendar"
	config := &models.VenueSourceConfig{
		VenueID:     s.venueID,
		CalendarURL: &url,
	}
	_, err := s.svc.CreateOrUpdate(config)
	s.NoError(err)

	hash := "h1"
	err = s.svc.UpdateAfterRun(s.venueID, &hash, nil, 20)
	s.NoError(err)

	result, _ := s.svc.GetByVenueID(s.venueID)
	s.Equal(10, result.EventsExpected) // (0 + 20) / 2

	hash = "h2"
	err = s.svc.UpdateAfterRun(s.venueID, &hash, nil, 20)
	s.NoError(err)

	result, _ = s.svc.GetByVenueID(s.venueID)
	s.Equal(15, result.EventsExpected) // (10 + 20) / 2
}

func (s *VenueSourceConfigIntegrationTestSuite) TestUpdateAfterRun_NotFound() {
	hash := "abc"
	err := s.svc.UpdateAfterRun(99999, &hash, nil, 5)
	s.Error(err)
	s.Contains(err.Error(), "not found")
}

func (s *VenueSourceConfigIntegrationTestSuite) TestUpdateAfterRun_ResetsFailures() {
	url := "https://testvenue.com/calendar"
	config := &models.VenueSourceConfig{
		VenueID:     s.venueID,
		CalendarURL: &url,
	}
	_, err := s.svc.CreateOrUpdate(config)
	s.NoError(err)

	// Increment failures a few times
	s.NoError(s.svc.IncrementFailures(s.venueID))
	s.NoError(s.svc.IncrementFailures(s.venueID))
	result, _ := s.svc.GetByVenueID(s.venueID)
	s.Equal(2, result.ConsecutiveFailures)

	// UpdateAfterRun should reset
	hash := "h1"
	s.NoError(s.svc.UpdateAfterRun(s.venueID, &hash, nil, 5))
	result, _ = s.svc.GetByVenueID(s.venueID)
	s.Equal(0, result.ConsecutiveFailures)
}

// --- IncrementFailures tests ---

func (s *VenueSourceConfigIntegrationTestSuite) TestIncrementFailures_Success() {
	url := "https://testvenue.com/calendar"
	config := &models.VenueSourceConfig{
		VenueID:     s.venueID,
		CalendarURL: &url,
	}
	_, err := s.svc.CreateOrUpdate(config)
	s.NoError(err)

	err = s.svc.IncrementFailures(s.venueID)
	s.NoError(err)

	result, _ := s.svc.GetByVenueID(s.venueID)
	s.Equal(1, result.ConsecutiveFailures)

	err = s.svc.IncrementFailures(s.venueID)
	s.NoError(err)

	result, _ = s.svc.GetByVenueID(s.venueID)
	s.Equal(2, result.ConsecutiveFailures)
}

func (s *VenueSourceConfigIntegrationTestSuite) TestIncrementFailures_NotFound() {
	err := s.svc.IncrementFailures(99999)
	s.Error(err)
	s.Contains(err.Error(), "not found")
}

// --- RecordRun tests ---

func (s *VenueSourceConfigIntegrationTestSuite) TestRecordRun_Success() {
	method := "static"
	source := "ai"
	hash := "abc123"
	status := 200

	run := &models.VenueExtractionRun{
		VenueID:         s.venueID,
		RenderMethod:    &method,
		PreferredSource: &source,
		EventsExtracted: 10,
		EventsImported:  8,
		ContentHash:     &hash,
		HTTPStatus:      &status,
		DurationMs:      1500,
	}

	err := s.svc.RecordRun(run)
	s.NoError(err)
	s.NotZero(run.ID)
}

func (s *VenueSourceConfigIntegrationTestSuite) TestRecordRun_VenueIDRequired() {
	run := &models.VenueExtractionRun{}
	err := s.svc.RecordRun(run)
	s.Error(err)
	s.Contains(err.Error(), "venue_id is required")
}

func (s *VenueSourceConfigIntegrationTestSuite) TestRecordRun_WithError() {
	errMsg := "connection timeout"
	status := 0
	run := &models.VenueExtractionRun{
		VenueID:    s.venueID,
		HTTPStatus: &status,
		Error:      &errMsg,
		DurationMs: 30000,
	}

	err := s.svc.RecordRun(run)
	s.NoError(err)
	s.NotZero(run.ID)
}

// --- GetRecentRuns tests ---

func (s *VenueSourceConfigIntegrationTestSuite) TestGetRecentRuns_OrderByRunAtDesc() {
	for i := 0; i < 3; i++ {
		run := &models.VenueExtractionRun{
			VenueID:         s.venueID,
			RunAt:           time.Now().Add(time.Duration(i) * time.Hour),
			EventsExtracted: i + 1,
			DurationMs:      100,
		}
		s.NoError(s.svc.RecordRun(run))
	}

	runs, err := s.svc.GetRecentRuns(s.venueID, 10)
	s.NoError(err)
	s.Len(runs, 3)
	// Most recent first
	s.Equal(3, runs[0].EventsExtracted)
	s.Equal(2, runs[1].EventsExtracted)
	s.Equal(1, runs[2].EventsExtracted)
}

func (s *VenueSourceConfigIntegrationTestSuite) TestGetRecentRuns_RespectsLimit() {
	for i := 0; i < 5; i++ {
		run := &models.VenueExtractionRun{
			VenueID:    s.venueID,
			DurationMs: 100,
		}
		s.NoError(s.svc.RecordRun(run))
	}

	runs, err := s.svc.GetRecentRuns(s.venueID, 2)
	s.NoError(err)
	s.Len(runs, 2)
}

func (s *VenueSourceConfigIntegrationTestSuite) TestGetRecentRuns_DefaultLimit() {
	runs, err := s.svc.GetRecentRuns(s.venueID, 0)
	s.NoError(err)
	s.NotNil(runs) // empty but not nil
}

func (s *VenueSourceConfigIntegrationTestSuite) TestGetRecentRuns_MaxLimit() {
	runs, err := s.svc.GetRecentRuns(s.venueID, 500)
	s.NoError(err)
	s.NotNil(runs)
}

// --- ListConfigured tests ---

func (s *VenueSourceConfigIntegrationTestSuite) TestListConfigured_Empty() {
	configs, err := s.svc.ListConfigured()
	s.NoError(err)
	s.Empty(configs)
}

func (s *VenueSourceConfigIntegrationTestSuite) TestListConfigured_WithConfigs() {
	url := "https://testvenue.com/calendar"
	config := &models.VenueSourceConfig{
		VenueID:     s.venueID,
		CalendarURL: &url,
	}
	_, err := s.svc.CreateOrUpdate(config)
	s.NoError(err)

	configs, err := s.svc.ListConfigured()
	s.NoError(err)
	s.Len(configs, 1)
	s.Equal(s.venueID, configs[0].VenueID)
	// Verify venue is preloaded
	s.Equal("Test Venue", configs[0].Venue.Name)
}

// --- ResetRenderMethod tests ---

func (s *VenueSourceConfigIntegrationTestSuite) TestResetRenderMethod_Success() {
	url := "https://testvenue.com/calendar"
	method := "dynamic"
	config := &models.VenueSourceConfig{
		VenueID:      s.venueID,
		CalendarURL:  &url,
		RenderMethod: &method,
	}
	_, err := s.svc.CreateOrUpdate(config)
	s.NoError(err)

	// Verify render_method is set
	result, err := s.svc.GetByVenueID(s.venueID)
	s.NoError(err)
	s.Equal(&method, result.RenderMethod)

	// Reset
	err = s.svc.ResetRenderMethod(s.venueID)
	s.NoError(err)

	// Verify render_method is nil
	result, err = s.svc.GetByVenueID(s.venueID)
	s.NoError(err)
	s.Nil(result.RenderMethod)
}

func (s *VenueSourceConfigIntegrationTestSuite) TestResetRenderMethod_NotFound() {
	err := s.svc.ResetRenderMethod(99999)
	s.Error(err)
	s.Contains(err.Error(), "not found")
}

// --- Cascade delete test ---

func (s *VenueSourceConfigIntegrationTestSuite) TestCascadeDelete_VenueDeleteCleansUp() {
	url := "https://testvenue.com/calendar"
	config := &models.VenueSourceConfig{
		VenueID:     s.venueID,
		CalendarURL: &url,
	}
	_, err := s.svc.CreateOrUpdate(config)
	s.NoError(err)

	run := &models.VenueExtractionRun{
		VenueID:    s.venueID,
		DurationMs: 100,
	}
	s.NoError(s.svc.RecordRun(run))

	// Delete the venue
	s.db.Exec("DELETE FROM venues WHERE id = ?", s.venueID)

	// Config and runs should be gone
	result, err := s.svc.GetByVenueID(s.venueID)
	s.NoError(err)
	s.Nil(result)

	runs, err := s.svc.GetRecentRuns(s.venueID, 10)
	s.NoError(err)
	s.Empty(runs)
}

// --- End-to-end workflow test ---

func (s *VenueSourceConfigIntegrationTestSuite) TestEndToEnd_ExtractionWorkflow() {
	// 1. Create config
	url := "https://testvenue.com/calendar"
	config := &models.VenueSourceConfig{
		VenueID:     s.venueID,
		CalendarURL: &url,
	}
	created, err := s.svc.CreateOrUpdate(config)
	s.NoError(err)
	s.NotNil(created)

	// 2. Simulate successful extraction
	hash := "abc123"
	etag := "\"etag-1\""
	s.NoError(s.svc.UpdateAfterRun(s.venueID, &hash, &etag, 10))

	// Record the run
	method := "static"
	source := "ai"
	status := 200
	run := &models.VenueExtractionRun{
		VenueID:         s.venueID,
		RenderMethod:    &method,
		PreferredSource: &source,
		EventsExtracted: 10,
		EventsImported:  8,
		ContentHash:     &hash,
		HTTPStatus:      &status,
		DurationMs:      1500,
	}
	s.NoError(s.svc.RecordRun(run))

	// 3. Simulate a failure
	s.NoError(s.svc.IncrementFailures(s.venueID))

	result, _ := s.svc.GetByVenueID(s.venueID)
	s.Equal(1, result.ConsecutiveFailures)

	// 4. Simulate recovery
	hash2 := "def456"
	s.NoError(s.svc.UpdateAfterRun(s.venueID, &hash2, nil, 12))

	result, _ = s.svc.GetByVenueID(s.venueID)
	s.Equal(0, result.ConsecutiveFailures)
	s.Equal(&hash2, result.LastContentHash)

	// 5. Verify run history
	runs, err := s.svc.GetRecentRuns(s.venueID, 10)
	s.NoError(err)
	s.Len(runs, 1)
}
