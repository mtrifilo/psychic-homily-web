package catalog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

// TestParseImportDate locks in the PSY-927 premise: backfill window bounds must
// parse whether they arrive date-only (the API form) or as a Postgres DATE-column
// round-trip ("...T00:00:00Z"). Preserved from the retired import-job suite — the
// helper (parseImportDate + normalizeDateString) outlived the import jobs.
func TestParseImportDate(t *testing.T) {
	// Date-only (the API request form).
	if d, err := parseImportDate("2026-03-02"); err != nil {
		t.Fatalf("date-only should parse, got %v", err)
	} else if got := d.Format("2006-01-02"); got != "2026-03-02" {
		t.Fatalf("expected 2026-03-02, got %s", got)
	}

	// The DATE-column round-trip form, as a defensive caller might still pass.
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

// TestSyncRunToResponse covers the radio_sync_runs → DTO mapping: backfill window
// timestamps render YYYY-MM-DD, the show fields populate only for show-scoped
// runs, and the categorized errors carry over.
func TestSyncRunToResponse(t *testing.T) {
	t.Run("station-scoped run omits show + window", func(t *testing.T) {
		run := &catalogm.RadioSyncRun{
			ID:        7,
			StationID: radioSyncStationID(3),
			Station:   catalogm.RadioStation{Name: "KEXP"},
			RunType:   catalogm.RadioSyncRunTypeFetch,
			Trigger:   catalogm.RadioSyncRunTriggerManual,
			Status:    catalogm.RadioSyncRunStatusRunning,
		}
		resp := syncRunToResponse(run)
		if resp.ID != 7 || resp.StationName != "KEXP" {
			t.Fatalf("unexpected base mapping: %+v", resp)
		}
		if resp.ShowID != nil || resp.ShowName != nil {
			t.Fatalf("station-scoped run should have nil show fields, got %+v", resp)
		}
		if resp.WindowStart != nil || resp.WindowEnd != nil {
			t.Fatalf("station-scoped run should have nil window, got %+v", resp)
		}
	})

	t.Run("backfill run maps show, window dates, errors", func(t *testing.T) {
		showID := uint(5)
		ws := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		we := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
		detail := "boom"
		run := &catalogm.RadioSyncRun{
			ID:               9,
			StationID:        radioSyncStationID(3),
			Station:          catalogm.RadioStation{Name: "WFMU"},
			ShowID:           &showID,
			Show:             &catalogm.RadioShow{Name: "Sample Show"},
			RunType:          catalogm.RadioSyncRunTypeBackfill,
			Trigger:          catalogm.RadioSyncRunTriggerManual,
			Status:           catalogm.RadioSyncRunStatusPartial,
			WindowStart:      &ws,
			WindowEnd:        &we,
			EpisodesImported: 4,
			PlaysImported:    40,
			PlaysMatched:     30,
			PlaysUnmatched:   10,
			Errors: []catalogm.RadioSyncRunError{
				{Category: catalogm.RadioSyncRunErrorParseError, Detail: &detail},
			},
		}
		resp := syncRunToResponse(run)
		if resp.ShowID == nil || *resp.ShowID != 5 {
			t.Fatalf("expected show id 5, got %+v", resp.ShowID)
		}
		if resp.ShowName == nil || *resp.ShowName != "Sample Show" {
			t.Fatalf("expected show name, got %+v", resp.ShowName)
		}
		if resp.WindowStart == nil || *resp.WindowStart != "2025-01-01" {
			t.Fatalf("expected window_start 2025-01-01, got %+v", resp.WindowStart)
		}
		if resp.WindowEnd == nil || *resp.WindowEnd != "2025-12-31" {
			t.Fatalf("expected window_end 2025-12-31, got %+v", resp.WindowEnd)
		}
		if resp.PlaysUnmatched != 10 {
			t.Fatalf("expected plays_unmatched 10, got %d", resp.PlaysUnmatched)
		}
		if len(resp.Errors) != 1 || resp.Errors[0].Category != catalogm.RadioSyncRunErrorParseError {
			t.Fatalf("unexpected errors mapping: %+v", resp.Errors)
		}
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type RadioSyncManualIntegrationTestSuite struct {
	suite.Suite
	testDB       *testutil.TestDatabase
	db           *gorm.DB
	radioService *RadioService
}

func (suite *RadioSyncManualIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.radioService = NewRadioService(suite.db)
}

func (suite *RadioSyncManualIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *RadioSyncManualIntegrationTestSuite) TearDownTest() {
	// Reverse dependency order.
	suite.db.Exec("DELETE FROM radio_sync_run_errors")
	suite.db.Exec("DELETE FROM radio_sync_runs")
	suite.db.Exec("DELETE FROM radio_station_health")
	suite.db.Exec("DELETE FROM radio_shows")
	suite.db.Exec("DELETE FROM radio_stations")
}

func TestRadioSyncManualIntegrationTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration tests in short mode")
	}
	suite.Run(t, new(RadioSyncManualIntegrationTestSuite))
}

// seedStation creates a minimal station row directly (no provider config — these
// tests exercise the sync-run lifecycle, not provider ingestion).
func (suite *RadioSyncManualIntegrationTestSuite) seedStation() *catalogm.RadioStation {
	station := &catalogm.RadioStation{Name: "Test Station", BroadcastType: "both"}
	suite.Require().NoError(suite.db.Create(station).Error)
	return station
}

// seedRunningRun opens a radio_sync_runs row in the 'running' state for a station.
func (suite *RadioSyncManualIntegrationTestSuite) seedRunningRun(stationID uint) *catalogm.RadioSyncRun {
	run := &catalogm.RadioSyncRun{
		StationID: radioSyncStationID(stationID),
		RunType:   catalogm.RadioSyncRunTypeFetch,
		Trigger:   catalogm.RadioSyncRunTriggerManual,
		Status:    catalogm.RadioSyncRunStatusRunning,
		StartedAt: time.Now(),
	}
	suite.Require().NoError(suite.db.Create(run).Error)
	return run
}

func (suite *RadioSyncManualIntegrationTestSuite) TestGetSyncRun_Success() {
	station := suite.seedStation()
	run := suite.seedRunningRun(station.ID)

	resp, err := suite.radioService.GetSyncRun(run.ID)
	suite.Require().NoError(err)
	suite.Equal(run.ID, resp.ID)
	suite.Equal("Test Station", resp.StationName)
	suite.Equal(catalogm.RadioSyncRunStatusRunning, resp.Status)
}

func (suite *RadioSyncManualIntegrationTestSuite) TestGetSyncRun_NotFound() {
	_, err := suite.radioService.GetSyncRun(999999)
	var radioErr *apperrors.RadioError
	suite.Require().ErrorAs(err, &radioErr)
	suite.Equal(apperrors.CodeRadioSyncRunNotFound, radioErr.Code)
}

func (suite *RadioSyncManualIntegrationTestSuite) TestCancelSyncRun_RunningTransitionsToCancelled() {
	station := suite.seedStation()
	run := suite.seedRunningRun(station.ID)

	suite.Require().NoError(suite.radioService.CancelSyncRun(run.ID))

	var reloaded catalogm.RadioSyncRun
	suite.Require().NoError(suite.db.First(&reloaded, run.ID).Error)
	suite.Equal(catalogm.RadioSyncRunStatusCancelled, reloaded.Status)
	suite.Require().NotNil(reloaded.FinishedAt, "cancel must set finished_at (lifecycle CHECK)")
}

func (suite *RadioSyncManualIntegrationTestSuite) TestCancelSyncRun_NotFound() {
	err := suite.radioService.CancelSyncRun(999999)
	var radioErr *apperrors.RadioError
	suite.Require().ErrorAs(err, &radioErr)
	suite.Equal(apperrors.CodeRadioSyncRunNotFound, radioErr.Code)
}

func (suite *RadioSyncManualIntegrationTestSuite) TestCancelSyncRun_AlreadyTerminalNotCancellable() {
	station := suite.seedStation()
	run := suite.seedRunningRun(station.ID)
	// Drive it to a terminal status out of band.
	now := time.Now()
	suite.Require().NoError(suite.db.Model(&catalogm.RadioSyncRun{}).Where("id = ?", run.ID).
		Updates(map[string]any{"status": catalogm.RadioSyncRunStatusSuccess, "finished_at": now}).Error)

	err := suite.radioService.CancelSyncRun(run.ID)
	var radioErr *apperrors.RadioError
	suite.Require().ErrorAs(err, &radioErr)
	suite.Equal(apperrors.CodeRadioSyncNotCancellable, radioErr.Code)
}

func (suite *RadioSyncManualIntegrationTestSuite) TestTriggerShowBackfill_ShowNotFound() {
	_, err := suite.radioService.TriggerShowBackfill(999999, "2025-01-01", "2025-12-31")
	var radioErr *apperrors.RadioError
	suite.Require().ErrorAs(err, &radioErr)
	suite.Equal(apperrors.CodeRadioShowNotFound, radioErr.Code)
}

func (suite *RadioSyncManualIntegrationTestSuite) TestTriggerStationSync_StationNotFound() {
	_, err := suite.radioService.TriggerStationSync(999999, catalogm.RadioSyncRunTypeFetch)
	var radioErr *apperrors.RadioError
	suite.Require().ErrorAs(err, &radioErr)
	suite.Equal(apperrors.CodeRadioStationNotFound, radioErr.Code)
}

func (suite *RadioSyncManualIntegrationTestSuite) TestTriggerStationSync_InvalidMode() {
	station := suite.seedStation()
	_, err := suite.radioService.TriggerStationSync(station.ID, "backfill")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "invalid station sync mode")
}

// TestTriggerShowBackfill_OpensRunAndCompletes exercises the async path end-to-end:
// the trigger returns a pollable handle (status=running) the instant the run row
// opens (startAsyncSync's OnRunOpened/select), then the run executes in the
// background goroutine and GetSyncRun observes it reach a terminal status with the
// imported counts. (PSY-1135, adversarial-review — startAsyncSync was untested.)
func (suite *RadioSyncManualIntegrationTestSuite) TestTriggerShowBackfill_OpensRunAndCompletes() {
	src := catalogm.PlaylistSourceKEXP
	station := &catalogm.RadioStation{Name: "Trigger Station", BroadcastType: "both", PlaylistSource: &src}
	suite.Require().NoError(suite.db.Create(station).Error)
	showExt := "trigger-show-ext"
	show := &catalogm.RadioShow{StationID: station.ID, Name: "Trigger Show", ExternalID: &showExt}
	suite.Require().NoError(suite.db.Create(show).Error)

	track := "T"
	suite.radioService.playlistProviderFactory = func(string) (RadioPlaylistProvider, error) {
		return &mockPlaylistProvider{
			fetchNewEpisodesFn: func(string, time.Time, time.Time) ([]RadioEpisodeImport, error) {
				return []RadioEpisodeImport{{ExternalID: "ep-1", ShowExternalID: showExt, AirDate: "2026-06-15"}}, nil
			},
			fetchPlaylistFn: func(string) ([]RadioPlayImport, error) {
				return []RadioPlayImport{{Position: 1, ArtistName: "A", TrackTitle: &track}}, nil
			},
		}, nil
	}
	defer func() { suite.radioService.playlistProviderFactory = nil }()

	resp, err := suite.radioService.TriggerShowBackfill(show.ID, "2026-06-01", "2026-06-30")
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID, "trigger must return a pollable run handle")
	suite.Equal(catalogm.RadioSyncRunTypeBackfill, resp.RunType)
	suite.Equal(catalogm.RadioSyncRunTriggerManual, resp.Trigger)
	suite.Require().NotNil(resp.ShowID)
	suite.Equal(show.ID, *resp.ShowID)

	// The run executes in a background goroutine; poll until terminal.
	var final *contracts.RadioSyncRunResponse
	suite.Require().Eventually(func() bool {
		r, e := suite.radioService.GetSyncRun(resp.ID)
		if e != nil {
			return false
		}
		final = r
		return r.Status == catalogm.RadioSyncRunStatusSuccess || r.Status == catalogm.RadioSyncRunStatusPartial
	}, 5*time.Second, 20*time.Millisecond, "the backfill run should complete in the background")
	suite.Equal(1, final.EpisodesImported)
	suite.Equal(1, final.PlaysImported)
}

func (suite *RadioSyncManualIntegrationTestSuite) TestTriggerGlobalRematch_OpensRunAndCompletes() {
	resp, err := suite.radioService.TriggerGlobalRematch(contracts.GlobalRematchRequest{})
	suite.Require().NoError(err)
	suite.Require().NotZero(resp.ID)
	suite.Equal(catalogm.RadioSyncRunTypeRematch, resp.RunType)
	suite.Equal(catalogm.RadioSyncRunStatusRunning, resp.Status)
	suite.Nil(resp.StationID)

	var final *contracts.RadioSyncRunResponse
	suite.Require().Eventually(func() bool {
		r, e := suite.radioService.GetSyncRun(resp.ID)
		if e != nil {
			return false
		}
		final = r
		return r.Status == catalogm.RadioSyncRunStatusSuccess
	}, 5*time.Second, 20*time.Millisecond, "global rematch run should complete")
	suite.NotNil(final)
}

func (suite *RadioSyncManualIntegrationTestSuite) TestTriggerGlobalRematch_RejectsConcurrentRun() {
	first, err := suite.radioService.TriggerGlobalRematch(contracts.GlobalRematchRequest{})
	suite.Require().NoError(err)
	suite.Require().NotZero(first.ID)

	_, err = suite.radioService.TriggerGlobalRematch(contracts.GlobalRematchRequest{})
	suite.Require().Error(err)
	var radioErr *apperrors.RadioError
	suite.Require().ErrorAs(err, &radioErr)
	suite.Equal(apperrors.CodeRadioRematchAlreadyRunning, radioErr.Code)
}
