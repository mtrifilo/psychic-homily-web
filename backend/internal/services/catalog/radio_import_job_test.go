package catalog

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestRadioService_NilDB_ImportJob(t *testing.T) {
	svc := &RadioService{db: nil}

	assertNilDBErr := func(fn func() error) {
		t.Helper()
		err := fn()
		if err == nil {
			t.Fatal("expected error for nil db, got nil")
		}
		if err.Error() != "database not initialized" {
			t.Fatalf("expected 'database not initialized', got %q", err.Error())
		}
	}

	assertNilDBErr(func() error {
		_, err := svc.CreateImportJob(1, "2025-01-01", "2025-12-31")
		return err
	})
	assertNilDBErr(func() error { return svc.StartImportJob(1) })
	assertNilDBErr(func() error { return svc.CancelImportJob(1) })
	assertNilDBErr(func() error {
		_, err := svc.GetImportJob(1)
		return err
	})
	assertNilDBErr(func() error {
		_, err := svc.ListImportJobs(1)
		return err
	})
	assertNilDBErr(func() error {
		_, err := svc.ListAllActiveJobs()
		return err
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type RadioImportJobIntegrationTestSuite struct {
	suite.Suite
	testDB       *testutil.TestDatabase
	db           *gorm.DB
	radioService *RadioService
}

func (suite *RadioImportJobIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.radioService = NewRadioService(suite.db)
}

func (suite *RadioImportJobIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *RadioImportJobIntegrationTestSuite) TearDownTest() {
	// Clean up tables in reverse dependency order
	suite.db.Exec("DELETE FROM radio_import_jobs")
	suite.db.Exec("DELETE FROM radio_plays")
	suite.db.Exec("DELETE FROM radio_episodes")
	suite.db.Exec("DELETE FROM radio_shows")
	suite.db.Exec("DELETE FROM radio_stations")
}

func TestRadioImportJobIntegrationTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration tests in short mode")
	}
	suite.Run(t, new(RadioImportJobIntegrationTestSuite))
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func (suite *RadioImportJobIntegrationTestSuite) createStation(name string) *contracts.RadioStationDetailResponse {
	station, err := suite.radioService.CreateStation(&contracts.CreateRadioStationRequest{
		Name: name, BroadcastType: "both",
	})
	suite.Require().NoError(err)
	return station
}

func (suite *RadioImportJobIntegrationTestSuite) createShow(stationID uint, name string) *contracts.RadioShowDetailResponse {
	show, err := suite.radioService.CreateShow(stationID, &contracts.CreateRadioShowRequest{Name: name})
	suite.Require().NoError(err)
	return show
}

// ─── CreateImportJob Tests ──────────────────────────────────────────────────

func (suite *RadioImportJobIntegrationTestSuite) TestCreateImportJob_Success() {
	station := suite.createStation("Test Station")
	show := suite.createShow(station.ID, "Test Show")

	job, err := suite.radioService.CreateImportJob(show.ID, "2025-01-01", "2025-06-30")
	suite.Require().NoError(err)
	suite.Require().NotNil(job)
	suite.Equal(show.ID, job.ShowID)
	suite.Equal(station.ID, job.StationID)
	suite.Equal("2025-01-01", job.Since)
	suite.Equal("2025-06-30", job.Until)
	suite.Equal(catalogm.RadioImportJobStatusPending, job.Status)
	suite.Equal(0, job.EpisodesFound)
	suite.Equal(0, job.EpisodesImported)
	suite.Equal(0, job.PlaysImported)
	suite.Equal(0, job.PlaysMatched)
	suite.Equal("Test Show", job.ShowName)
	suite.Equal("Test Station", job.StationName)
}

func (suite *RadioImportJobIntegrationTestSuite) TestCreateImportJob_DuplicateRunning() {
	station := suite.createStation("Test Station")
	show := suite.createShow(station.ID, "Test Show")

	// Create first job
	_, err := suite.radioService.CreateImportJob(show.ID, "2025-01-01", "2025-06-30")
	suite.Require().NoError(err)

	// Attempt to create a second job — should fail
	_, err = suite.radioService.CreateImportJob(show.ID, "2025-07-01", "2025-12-31")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "already running or pending")
}

func (suite *RadioImportJobIntegrationTestSuite) TestCreateImportJob_ShowNotFound() {
	_, err := suite.radioService.CreateImportJob(99999, "2025-01-01", "2025-12-31")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "show not found")
}

func (suite *RadioImportJobIntegrationTestSuite) TestCreateImportJob_InvalidSinceDate() {
	station := suite.createStation("Test Station")
	show := suite.createShow(station.ID, "Test Show")

	_, err := suite.radioService.CreateImportJob(show.ID, "not-a-date", "2025-12-31")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "invalid since date")
}

func (suite *RadioImportJobIntegrationTestSuite) TestCreateImportJob_InvalidUntilDate() {
	station := suite.createStation("Test Station")
	show := suite.createShow(station.ID, "Test Show")

	_, err := suite.radioService.CreateImportJob(show.ID, "2025-01-01", "not-a-date")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "invalid until date")
}

// ─── CancelImportJob Tests ─────────────────────────────────────────────────

func (suite *RadioImportJobIntegrationTestSuite) TestCancelImportJob_Success() {
	station := suite.createStation("Test Station")
	show := suite.createShow(station.ID, "Test Show")

	job, err := suite.radioService.CreateImportJob(show.ID, "2025-01-01", "2025-06-30")
	suite.Require().NoError(err)

	err = suite.radioService.CancelImportJob(job.ID)
	suite.Require().NoError(err)

	// Verify status changed
	updated, err := suite.radioService.GetImportJob(job.ID)
	suite.Require().NoError(err)
	suite.Equal(catalogm.RadioImportJobStatusCancelled, updated.Status)
	suite.NotNil(updated.CompletedAt)
}

func (suite *RadioImportJobIntegrationTestSuite) TestCancelImportJob_NotFound() {
	err := suite.radioService.CancelImportJob(99999)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "job not found")
}

func (suite *RadioImportJobIntegrationTestSuite) TestCancelImportJob_AlreadyCompleted() {
	station := suite.createStation("Test Station")
	show := suite.createShow(station.ID, "Test Show")

	job, err := suite.radioService.CreateImportJob(show.ID, "2025-01-01", "2025-06-30")
	suite.Require().NoError(err)

	// Manually set to completed
	suite.db.Model(&catalogm.RadioImportJob{}).Where("id = ?", job.ID).
		Update("status", catalogm.RadioImportJobStatusCompleted)

	err = suite.radioService.CancelImportJob(job.ID)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "cannot be cancelled")
}

// ─── GetImportJob Tests ────────────────────────────────────────────────────

func (suite *RadioImportJobIntegrationTestSuite) TestGetImportJob_Success() {
	station := suite.createStation("Test Station")
	show := suite.createShow(station.ID, "Test Show")

	created, err := suite.radioService.CreateImportJob(show.ID, "2025-01-01", "2025-06-30")
	suite.Require().NoError(err)

	job, err := suite.radioService.GetImportJob(created.ID)
	suite.Require().NoError(err)
	suite.Equal(created.ID, job.ID)
	suite.Equal("Test Show", job.ShowName)
	suite.Equal("Test Station", job.StationName)
}

func (suite *RadioImportJobIntegrationTestSuite) TestGetImportJob_NotFound() {
	_, err := suite.radioService.GetImportJob(99999)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "job not found")
}

// ─── ListImportJobs Tests ──────────────────────────────────────────────────

func (suite *RadioImportJobIntegrationTestSuite) TestListImportJobs_Success() {
	station := suite.createStation("Test Station")
	show := suite.createShow(station.ID, "Test Show")

	// Create a job, cancel it, then create another
	job1, err := suite.radioService.CreateImportJob(show.ID, "2025-01-01", "2025-03-31")
	suite.Require().NoError(err)

	err = suite.radioService.CancelImportJob(job1.ID)
	suite.Require().NoError(err)

	_, err = suite.radioService.CreateImportJob(show.ID, "2025-04-01", "2025-06-30")
	suite.Require().NoError(err)

	jobs, err := suite.radioService.ListImportJobs(show.ID)
	suite.Require().NoError(err)
	suite.Len(jobs, 2)
	// Most recent first
	suite.Equal("2025-04-01", jobs[0].Since)
	suite.Equal("2025-01-01", jobs[1].Since)
}

func (suite *RadioImportJobIntegrationTestSuite) TestListImportJobs_Empty() {
	station := suite.createStation("Test Station")
	show := suite.createShow(station.ID, "Test Show")

	jobs, err := suite.radioService.ListImportJobs(show.ID)
	suite.Require().NoError(err)
	suite.Len(jobs, 0)
}

// ─── ListAllActiveJobs Tests ───────────────────────────────────────────────

func (suite *RadioImportJobIntegrationTestSuite) TestListAllActiveJobs_Success() {
	station := suite.createStation("Test Station")
	show1 := suite.createShow(station.ID, "Show 1")
	show2 := suite.createShow(station.ID, "Show 2")

	// Create one pending job for each show
	_, err := suite.radioService.CreateImportJob(show1.ID, "2025-01-01", "2025-06-30")
	suite.Require().NoError(err)
	_, err = suite.radioService.CreateImportJob(show2.ID, "2025-01-01", "2025-06-30")
	suite.Require().NoError(err)

	jobs, err := suite.radioService.ListAllActiveJobs()
	suite.Require().NoError(err)
	suite.Len(jobs, 2)
}

// ─── CreateImportJob allows after cancellation ─────────────────────────────

func (suite *RadioImportJobIntegrationTestSuite) TestCreateImportJob_AllowedAfterCancellation() {
	station := suite.createStation("Test Station")
	show := suite.createShow(station.ID, "Test Show")

	// Create and cancel
	job, err := suite.radioService.CreateImportJob(show.ID, "2025-01-01", "2025-06-30")
	suite.Require().NoError(err)
	err = suite.radioService.CancelImportJob(job.ID)
	suite.Require().NoError(err)

	// Now creating a new job should succeed
	_, err = suite.radioService.CreateImportJob(show.ID, "2025-07-01", "2025-12-31")
	suite.Require().NoError(err)
}
