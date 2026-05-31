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

func TestParseImportDate(t *testing.T) {
	// Date-only (the API request form).
	if d, err := parseImportDate("2026-03-02"); err != nil {
		t.Fatalf("date-only should parse, got %v", err)
	} else if got := d.Format("2006-01-02"); got != "2026-03-02" {
		t.Fatalf("expected 2026-03-02, got %s", got)
	}

	// PSY-927: the DATE-column round-trip form, as read back from a persisted
	// import job. This exact value previously failed every auto-backfill job
	// with `parsing time "...": extra text "T00:00:00Z"`.
	if d, err := parseImportDate("2026-03-02T00:00:00Z"); err != nil {
		t.Fatalf("DATE round-trip form should parse, got %v", err)
	} else if got := d.Format("2006-01-02"); got != "2026-03-02" {
		t.Fatalf("expected 2026-03-02, got %s", got)
	}

	// Genuinely malformed input still errors.
	if _, err := parseImportDate("not-a-date"); err == nil {
		t.Fatal("expected error for invalid date, got nil")
	}
}

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

// TestImportJob_PersistedDatesParse locks in the PSY-927 premise end-to-end:
// since/until persist in Postgres DATE columns and round-trip through GORM with
// a time component, and the value reloaded the way runImportJob reads it must
// parse via parseImportDate. The helper-only TestParseImportDate hard-codes the
// "T00:00:00Z" form; this catches a future driver/GORM change to that rendering
// that the helper test would miss (which is exactly how the original bug broke
// every auto-backfill job).
func (suite *RadioImportJobIntegrationTestSuite) TestImportJob_PersistedDatesParse() {
	station := suite.createStation("Test Station")
	show := suite.createShow(station.ID, "Test Show")

	job, err := suite.radioService.CreateImportJob(show.ID, "2026-03-02", "2026-03-31")
	suite.Require().NoError(err)

	var reloaded catalogm.RadioImportJob
	suite.Require().NoError(suite.db.First(&reloaded, job.ID).Error)

	since, serr := parseImportDate(reloaded.Since)
	suite.Require().NoError(serr, "persisted Since %q must parse", reloaded.Since)
	suite.Equal("2026-03-02", since.Format("2006-01-02"))

	until, uerr := parseImportDate(reloaded.Until)
	suite.Require().NoError(uerr, "persisted Until %q must parse", reloaded.Until)
	suite.Equal("2026-03-31", until.Format("2006-01-02"))
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

// ─── StartImportJob Tests ──────────────────────────────────────────────────

// TestStartImportJob_RejectsAlreadyRunning verifies that StartImportJob refuses
// to start a job whose status is already 'running'. The conditional UPDATE on
// (status = pending) must fail with RowsAffected == 0 and surface the actual
// current status without spawning a duplicate runImportJob goroutine.
func (suite *RadioImportJobIntegrationTestSuite) TestStartImportJob_RejectsAlreadyRunning() {
	station := suite.createStation("Test Station")
	show := suite.createShow(station.ID, "Test Show")

	job, err := suite.radioService.CreateImportJob(show.ID, "2025-01-01", "2025-06-30")
	suite.Require().NoError(err)

	// Pretend a prior StartImportJob call already won the race.
	suite.db.Model(&catalogm.RadioImportJob{}).Where("id = ?", job.ID).
		Update("status", catalogm.RadioImportJobStatusRunning)

	err = suite.radioService.StartImportJob(job.ID)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "not in pending status")
	suite.Contains(err.Error(), catalogm.RadioImportJobStatusRunning)
}

// TestStartImportJob_NotFound verifies the not-found path returns a clear
// "job not found" error before reaching the conditional UPDATE.
func (suite *RadioImportJobIntegrationTestSuite) TestStartImportJob_NotFound() {
	err := suite.radioService.StartImportJob(99999)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "job not found")
}

// TestStartImportJob_RaceOnlyOneWins simulates two concurrent StartImportJob
// callers for the same pending job and verifies exactly one wins. This is the
// core race-condition guard: without the conditional UPDATE both callers could
// pass an unguarded check-then-act, transition the job to RUNNING, and spawn
// duplicate runImportJob goroutines that would import the same episodes twice.
//
// The winner does spawn a runImportJob goroutine via shared.GoSafe, but with a
// no-PlaylistSource test station that goroutine fails fast inside
// importShowEpisodesWithProgress and calls failJob, so the final status is
// FAILED rather than RUNNING. The race-condition assertion lives in the
// success/failure counts (1/1) — the post-state is captured loosely.
func (suite *RadioImportJobIntegrationTestSuite) TestStartImportJob_RaceOnlyOneWins() {
	station := suite.createStation("Test Station")
	show := suite.createShow(station.ID, "Test Show")

	job, err := suite.radioService.CreateImportJob(show.ID, "2025-01-01", "2025-06-30")
	suite.Require().NoError(err)

	// Two parallel starters race on the same job.
	type startResult struct{ err error }
	results := make(chan startResult, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			results <- startResult{err: suite.radioService.StartImportJob(job.ID)}
		}()
	}
	close(start)

	var successes, failures int
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err == nil {
			successes++
		} else {
			failures++
			suite.Contains(r.err.Error(), "not in pending status",
				"loser should report status-precondition failure")
		}
	}
	suite.Equal(1, successes, "exactly one caller should win")
	suite.Equal(1, failures, "exactly one caller should lose")

	// The winner spawned exactly one runImportJob goroutine. With a test
	// station that has no PlaylistSource, that goroutine fails fast inside
	// importShowEpisodesWithProgress and the job ends up FAILED. The job
	// MUST NOT remain in pending — the conditional UPDATE always fired once.
	updated, err := suite.radioService.GetImportJob(job.ID)
	suite.Require().NoError(err)
	suite.NotEqual(catalogm.RadioImportJobStatusPending, updated.Status,
		"job should have transitioned out of pending exactly once")
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
