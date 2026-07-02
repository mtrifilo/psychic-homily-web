package catalog

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/utils"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func assertNilDBError(t *testing.T, fn func() error) {
	t.Helper()
	err := fn()
	if err == nil {
		t.Fatal("expected error for nil db, got nil")
	}
	if err.Error() != "database not initialized" {
		t.Fatalf("expected 'database not initialized', got %q", err.Error())
	}
}

func TestRadioService_NilDB_Station(t *testing.T) {
	svc := &RadioService{db: nil}
	assertNilDBError(t, func() error {
		_, err := svc.CreateStation(&contracts.CreateRadioStationRequest{Name: "x", BroadcastType: "internet"})
		return err
	})
	assertNilDBError(t, func() error { _, err := svc.GetStation(1); return err })
	assertNilDBError(t, func() error { _, err := svc.GetStationBySlug("x"); return err })
	assertNilDBError(t, func() error { _, err := svc.ListStations(nil); return err })
	assertNilDBError(t, func() error {
		_, err := svc.UpdateStation(1, &contracts.UpdateRadioStationRequest{})
		return err
	})
	assertNilDBError(t, func() error { return svc.DeleteStation(1) })
}

func TestRadioService_NilDB_Show(t *testing.T) {
	svc := &RadioService{db: nil}
	assertNilDBError(t, func() error {
		_, err := svc.CreateShow(1, &contracts.CreateRadioShowRequest{Name: "x"})
		return err
	})
	assertNilDBError(t, func() error { _, err := svc.GetShow(1); return err })
	assertNilDBError(t, func() error { _, err := svc.GetShowBySlug("x"); return err })
	assertNilDBError(t, func() error { _, err := svc.ListShows(1, ""); return err })
	assertNilDBError(t, func() error {
		_, err := svc.UpdateShow(1, &contracts.UpdateRadioShowRequest{})
		return err
	})
	assertNilDBError(t, func() error { return svc.DeleteShow(1) })
}

func TestRadioService_NilDB_Episode(t *testing.T) {
	svc := &RadioService{db: nil}
	assertNilDBError(t, func() error { _, _, err := svc.GetEpisodes(1, 10, 0); return err })
	assertNilDBError(t, func() error { _, err := svc.GetEpisodeByShowAndDate(1, "2026-01-01"); return err })
	assertNilDBError(t, func() error { _, err := svc.GetEpisodeDetail(1); return err })
}

func TestRadioService_NilDB_Aggregation(t *testing.T) {
	svc := &RadioService{db: nil}
	assertNilDBError(t, func() error { _, err := svc.GetTopArtistsForShow(1, 90, 10); return err })
	assertNilDBError(t, func() error { _, err := svc.GetTopLabelsForShow(1, 90, 10); return err })
	assertNilDBError(t, func() error { _, err := svc.GetAsHeardOnForArtist(1); return err })
	assertNilDBError(t, func() error { _, err := svc.GetAsHeardOnForRelease(1); return err })
	assertNilDBError(t, func() error { _, err := svc.GetNewReleaseRadar(0, 10); return err })
	assertNilDBError(t, func() error { _, err := svc.GetRadioStats(); return err })
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type RadioServiceIntegrationTestSuite struct {
	suite.Suite
	testDB       *testutil.TestDatabase
	db           *gorm.DB
	radioService *RadioService
}

func (suite *RadioServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.radioService = &RadioService{db: suite.testDB.DB}
}

func (suite *RadioServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

// SetupTest wipes data so every test starts from a known empty state.
// Migrations seed WFMU network + 4 stations on the test container; without
// SetupTest, the first test would inherit those rows.
func (suite *RadioServiceIntegrationTestSuite) SetupTest() {
	suite.cleanupRadioTables()
}

func (suite *RadioServiceIntegrationTestSuite) TearDownTest() {
	suite.cleanupRadioTables()
}

func (suite *RadioServiceIntegrationTestSuite) cleanupRadioTables() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM radio_artist_affinity")
	_, _ = sqlDB.Exec("DELETE FROM radio_plays")
	_, _ = sqlDB.Exec("DELETE FROM radio_episodes")
	_, _ = sqlDB.Exec("DELETE FROM radio_shows")
	// stations FK -> networks is ON DELETE SET NULL, so order is flexible,
	// but we delete stations before networks for clarity.
	_, _ = sqlDB.Exec("DELETE FROM radio_stations")
	_, _ = sqlDB.Exec("DELETE FROM radio_networks")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM labels")
}

func TestRadioServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(RadioServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *RadioServiceIntegrationTestSuite) createStation(name string) *contracts.RadioStationDetailResponse {
	resp, err := suite.radioService.CreateStation(&contracts.CreateRadioStationRequest{
		Name:          name,
		BroadcastType: catalogm.BroadcastTypeBoth,
	})
	suite.Require().NoError(err)
	return resp
}

func (suite *RadioServiceIntegrationTestSuite) createShow(stationID uint, name string) *contracts.RadioShowDetailResponse {
	resp, err := suite.radioService.CreateShow(stationID, &contracts.CreateRadioShowRequest{
		Name: name,
	})
	suite.Require().NoError(err)
	return resp
}

func (suite *RadioServiceIntegrationTestSuite) createEpisode(showID uint, airDate string) *catalogm.RadioEpisode {
	ep := &catalogm.RadioEpisode{
		ShowID:  showID,
		AirDate: airDate,
	}
	err := suite.db.Create(ep).Error
	suite.Require().NoError(err)
	return ep
}

// createEpisodeWindowed seeds an episode with an explicit frozen air window and
// play_count, for exercising the PSY-1285 air-window feed gate: a not-yet-aired
// episode has starts > now; a windowless 0-track row passes nil window + 0 plays.
func (suite *RadioServiceIntegrationTestSuite) createEpisodeWindowed(showID uint, airDate string, starts, ends *time.Time, playCount int) *catalogm.RadioEpisode {
	ep := &catalogm.RadioEpisode{
		ShowID:    showID,
		AirDate:   airDate,
		StartsAt:  starts,
		EndsAt:    ends,
		PlayCount: playCount,
	}
	suite.Require().NoError(suite.db.Create(ep).Error)
	return ep
}

// createAiredEpisode seeds a 0-track episode whose window has already passed, so it
// surfaces in the feed past the PSY-1285 air-window gate. For tests about feed
// SCOPING / ordering (not the gate itself), where the episode just needs to appear.
func (suite *RadioServiceIntegrationTestSuite) createAiredEpisode(showID uint, airDate string) *catalogm.RadioEpisode {
	now := time.Now().UTC()
	starts := now.Add(-72 * time.Hour)
	ends := now.Add(-71 * time.Hour)
	return suite.createEpisodeWindowed(showID, airDate, &starts, &ends, 0)
}

func (suite *RadioServiceIntegrationTestSuite) createPlay(episodeID uint, position int, artistName string) *catalogm.RadioPlay {
	play := &catalogm.RadioPlay{
		EpisodeID:  episodeID,
		Position:   position,
		ArtistName: artistName,
	}
	err := suite.db.Create(play).Error
	suite.Require().NoError(err)
	return play
}

func (suite *RadioServiceIntegrationTestSuite) createArtist(name string) *catalogm.Artist {
	slug := utils.GenerateArtistSlug(name)
	artist := &catalogm.Artist{Name: name, Slug: &slug}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *RadioServiceIntegrationTestSuite) createRelease(title string) *catalogm.Release {
	release := &catalogm.Release{Title: title}
	err := suite.db.Create(release).Error
	suite.Require().NoError(err)
	return release
}

// =============================================================================
// STATION CRUD TESTS
// =============================================================================

func (suite *RadioServiceIntegrationTestSuite) TestCreateStation_Success() {
	city := "Seattle"
	state := "WA"
	freq := 90.3
	resp, err := suite.radioService.CreateStation(&contracts.CreateRadioStationRequest{
		Name:          "KEXP",
		BroadcastType: catalogm.BroadcastTypeBoth,
		City:          &city,
		State:         &state,
		FrequencyMHz:  &freq,
	})

	suite.Require().NoError(err)
	suite.NotZero(resp.ID)
	suite.Equal("KEXP", resp.Name)
	suite.Equal("kexp", resp.Slug)
	suite.Equal("both", resp.BroadcastType)
	suite.Equal(&city, resp.City)
	suite.Equal(&state, resp.State)
	suite.Equal(&freq, resp.FrequencyMHz)
	suite.True(resp.IsActive)
	suite.Equal(0, resp.ShowCount)
}

func (suite *RadioServiceIntegrationTestSuite) TestCreateStation_CustomSlug() {
	resp, err := suite.radioService.CreateStation(&contracts.CreateRadioStationRequest{
		Name:          "KEXP",
		Slug:          "kexp-seattle",
		BroadcastType: catalogm.BroadcastTypeInternet,
	})

	suite.Require().NoError(err)
	suite.Equal("kexp-seattle", resp.Slug)
}

func (suite *RadioServiceIntegrationTestSuite) TestCreateStation_InvalidBroadcastType() {
	_, err := suite.radioService.CreateStation(&contracts.CreateRadioStationRequest{
		Name:          "Bad Station",
		BroadcastType: "satellite",
	})

	suite.Error(err)
	suite.Contains(err.Error(), "invalid broadcast type")
}

func (suite *RadioServiceIntegrationTestSuite) TestCreateStation_InvalidPlaylistSource() {
	// PSY-927: reject the bad value that broke WFMU import. A valid broadcast
	// type is supplied so the failure is attributable to playlist_source.
	bad := "wfmu_html"
	_, err := suite.radioService.CreateStation(&contracts.CreateRadioStationRequest{
		Name:           "Bad Source Station",
		BroadcastType:  catalogm.BroadcastTypeInternet,
		PlaylistSource: &bad,
	})

	suite.Error(err)
	suite.Contains(err.Error(), "invalid playlist source")
}

func (suite *RadioServiceIntegrationTestSuite) TestCreateStation_DuplicateNameRejected() {
	suite.createStation("KEXP")

	// Station names are unique (case-insensitive). A duplicate is rejected with
	// a clean conflict error rather than silently creating a slug-suffixed dupe
	// (PSY-1131). Shows, by contrast, are intentionally not name-unique.
	_, err := suite.radioService.CreateStation(&contracts.CreateRadioStationRequest{
		Name:          "KEXP",
		BroadcastType: catalogm.BroadcastTypeInternet,
	})

	suite.Require().Error(err)
	var radioErr *apperrors.RadioError
	suite.ErrorAs(err, &radioErr)
	suite.Equal(apperrors.CodeRadioStationNameConflict, radioErr.Code)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetStation_Success() {
	created := suite.createStation("KEXP")

	resp, err := suite.radioService.GetStation(created.ID)

	suite.Require().NoError(err)
	suite.Equal(created.ID, resp.ID)
	suite.Equal("KEXP", resp.Name)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetStation_NotFound() {
	_, err := suite.radioService.GetStation(99999)

	suite.Error(err)
	var radioErr *apperrors.RadioError
	suite.ErrorAs(err, &radioErr)
	suite.Equal(apperrors.CodeRadioStationNotFound, radioErr.Code)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetStationBySlug_Success() {
	suite.createStation("KEXP")

	resp, err := suite.radioService.GetStationBySlug("kexp")

	suite.Require().NoError(err)
	suite.Equal("KEXP", resp.Name)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetStationBySlug_NotFound() {
	_, err := suite.radioService.GetStationBySlug("nonexistent")

	suite.Error(err)
	var radioErr *apperrors.RadioError
	suite.ErrorAs(err, &radioErr)
}

func (suite *RadioServiceIntegrationTestSuite) TestListStations_All() {
	suite.createStation("KEXP")
	suite.createStation("WFMU")

	resp, err := suite.radioService.ListStations(map[string]interface{}{})

	suite.Require().NoError(err)
	suite.Len(resp, 2)
	// Ordered by name ASC
	suite.Equal("KEXP", resp[0].Name)
	suite.Equal("WFMU", resp[1].Name)
}

func (suite *RadioServiceIntegrationTestSuite) TestListStations_WithShowCounts() {
	station := suite.createStation("KEXP")
	suite.createShow(station.ID, "Morning Show")
	suite.createShow(station.ID, "Afternoon Show")

	resp, err := suite.radioService.ListStations(map[string]interface{}{})

	suite.Require().NoError(err)
	suite.Len(resp, 1)
	suite.Equal(2, resp[0].ShowCount)
}

func (suite *RadioServiceIntegrationTestSuite) TestUpdateStation_Success() {
	station := suite.createStation("KEXP")

	newName := "KEXP 90.3"
	city := "Seattle"
	resp, err := suite.radioService.UpdateStation(station.ID, &contracts.UpdateRadioStationRequest{
		Name: &newName,
		City: &city,
	})

	suite.Require().NoError(err)
	suite.Equal("KEXP 90.3", resp.Name)
	suite.Equal(&city, resp.City)
}

func (suite *RadioServiceIntegrationTestSuite) TestUpdateStation_InvalidBroadcastType() {
	station := suite.createStation("KEXP")

	bad := "satellite"
	_, err := suite.radioService.UpdateStation(station.ID, &contracts.UpdateRadioStationRequest{
		BroadcastType: &bad,
	})

	suite.Error(err)
	suite.Contains(err.Error(), "invalid broadcast type")
}

func (suite *RadioServiceIntegrationTestSuite) TestUpdateStation_InvalidPlaylistSource() {
	// PSY-927: an existing station can't be updated to the invalid value either.
	station := suite.createStation("KEXP")

	bad := "wfmu_html"
	_, err := suite.radioService.UpdateStation(station.ID, &contracts.UpdateRadioStationRequest{
		PlaylistSource: &bad,
	})

	suite.Error(err)
	suite.Contains(err.Error(), "invalid playlist source")
}

func (suite *RadioServiceIntegrationTestSuite) TestUpdateStation_NotFound() {
	newName := "Gone"
	_, err := suite.radioService.UpdateStation(99999, &contracts.UpdateRadioStationRequest{Name: &newName})

	suite.Error(err)
	var radioErr *apperrors.RadioError
	suite.ErrorAs(err, &radioErr)
}

func (suite *RadioServiceIntegrationTestSuite) TestDeleteStation_Success() {
	station := suite.createStation("KEXP")

	err := suite.radioService.DeleteStation(station.ID)

	suite.NoError(err)
	_, err = suite.radioService.GetStation(station.ID)
	suite.Error(err)
}

func (suite *RadioServiceIntegrationTestSuite) TestDeleteStation_NotFound() {
	err := suite.radioService.DeleteStation(99999)

	suite.Error(err)
	var radioErr *apperrors.RadioError
	suite.ErrorAs(err, &radioErr)
}

// =============================================================================
// SHOW CRUD TESTS
// =============================================================================

func (suite *RadioServiceIntegrationTestSuite) TestCreateShow_Success() {
	station := suite.createStation("KEXP")

	hostName := "John Richards"
	resp, err := suite.radioService.CreateShow(station.ID, &contracts.CreateRadioShowRequest{
		Name:     "The Morning Show",
		HostName: &hostName,
	})

	suite.Require().NoError(err)
	suite.NotZero(resp.ID)
	suite.Equal("The Morning Show", resp.Name)
	suite.Equal("the-morning-show", resp.Slug)
	suite.Equal(&hostName, resp.HostName)
	suite.Equal(station.ID, resp.StationID)
	suite.Equal("KEXP", resp.StationName)
	suite.True(resp.IsActive)
}

func (suite *RadioServiceIntegrationTestSuite) TestCreateShow_StationNotFound() {
	_, err := suite.radioService.CreateShow(99999, &contracts.CreateRadioShowRequest{
		Name: "Orphan Show",
	})

	suite.Error(err)
	var radioErr *apperrors.RadioError
	suite.ErrorAs(err, &radioErr)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetShow_Success() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")

	resp, err := suite.radioService.GetShow(show.ID)

	suite.Require().NoError(err)
	suite.Equal("Morning Show", resp.Name)
	suite.Equal("KEXP", resp.StationName)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetShow_NotFound() {
	_, err := suite.radioService.GetShow(99999)

	suite.Error(err)
	var radioErr *apperrors.RadioError
	suite.ErrorAs(err, &radioErr)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetShowBySlug_Success() {
	station := suite.createStation("KEXP")
	suite.createShow(station.ID, "Morning Show")

	resp, err := suite.radioService.GetShowBySlug("morning-show")

	suite.Require().NoError(err)
	suite.Equal("Morning Show", resp.Name)
}

func (suite *RadioServiceIntegrationTestSuite) TestListShows_WithEpisodeCounts() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	suite.createEpisode(show.ID, "2026-01-01")
	suite.createEpisode(show.ID, "2026-01-02")
	suite.createShow(station.ID, "Afternoon Show") // no episodes

	resp, err := suite.radioService.ListShows(station.ID, "")

	suite.Require().NoError(err)
	suite.Len(resp, 2)
	// Ordered by name ASC
	suite.Equal("Afternoon Show", resp[0].Name)
	suite.Equal(int64(0), resp[0].EpisodeCount)
	suite.Equal("Morning Show", resp[1].Name)
	suite.Equal(int64(2), resp[1].EpisodeCount)
}

// PSY-1193: schedule_locked is emitted in list rows so the admin UI can badge pinned
// (hand-curated) shows vs scrape-managed ones at a glance. Defaults false; flips to true
// when an admin locks the schedule via UpdateShow.
func (suite *RadioServiceIntegrationTestSuite) TestListShows_SurfacesScheduleLocked() {
	station := suite.createStation("WFMU")
	locked := suite.createShow(station.ID, "Locked Show")
	suite.createShow(station.ID, "Auto Show")

	lockedFlag := true
	_, err := suite.radioService.UpdateShow(locked.ID, &contracts.UpdateRadioShowRequest{ScheduleLocked: &lockedFlag})
	suite.Require().NoError(err)

	resp, err := suite.radioService.ListShows(station.ID, "")
	suite.Require().NoError(err)
	suite.Require().Len(resp, 2)
	// Ordered by name ASC: "Auto Show" then "Locked Show".
	suite.Equal("Auto Show", resp[0].Name)
	suite.False(resp[0].ScheduleLocked, "unlocked show defaults to false in list rows")
	suite.Equal("Locked Show", resp[1].Name)
	suite.True(resp[1].ScheduleLocked, "locked show surfaces schedule_locked=true in list rows")
}

func (suite *RadioServiceIntegrationTestSuite) TestUpdateShow_Success() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")

	newName := "KEXP Morning Show"
	host := "John Richards"
	resp, err := suite.radioService.UpdateShow(show.ID, &contracts.UpdateRadioShowRequest{
		Name:     &newName,
		HostName: &host,
	})

	suite.Require().NoError(err)
	suite.Equal("KEXP Morning Show", resp.Name)
	suite.Equal(&host, resp.HostName)
}

func (suite *RadioServiceIntegrationTestSuite) TestUpdateShow_NotFound() {
	newName := "Gone"
	_, err := suite.radioService.UpdateShow(99999, &contracts.UpdateRadioShowRequest{Name: &newName})

	suite.Error(err)
}

// PSY-1172: lifecycle_state is now writable via UpdateShow (the only write path for it).
// 'retired' is the manual-only "ended forever" signal the nightly janitor never sets.
func (suite *RadioServiceIntegrationTestSuite) TestUpdateShow_SetLifecycleState_Retired() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	suite.Equal(catalogm.RadioLifecycleActive, show.LifecycleState, "new shows default to active")

	retired := catalogm.RadioLifecycleRetired
	resp, err := suite.radioService.UpdateShow(show.ID, &contracts.UpdateRadioShowRequest{
		LifecycleState: &retired,
	})

	suite.Require().NoError(err)
	suite.Equal(catalogm.RadioLifecycleRetired, resp.LifecycleState)
}

func (suite *RadioServiceIntegrationTestSuite) TestUpdateShow_SetLifecycleState_Dormant() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")

	dormant := catalogm.RadioLifecycleDormant
	resp, err := suite.radioService.UpdateShow(show.ID, &contracts.UpdateRadioShowRequest{
		LifecycleState: &dormant,
	})

	suite.Require().NoError(err)
	suite.Equal(catalogm.RadioLifecycleDormant, resp.LifecycleState)
}

// An invalid lifecycle_state is rejected with a typed validation error and writes
// nothing — even other fields in the same request must not be persisted (the guard
// runs before the DB write).
func (suite *RadioServiceIntegrationTestSuite) TestUpdateShow_LifecycleStateInvalid_NoWrite() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")

	bogus := "archived"
	newName := "Renamed Show"
	_, err := suite.radioService.UpdateShow(show.ID, &contracts.UpdateRadioShowRequest{
		Name:           &newName,
		LifecycleState: &bogus,
	})

	suite.Require().Error(err)
	var radioErr *apperrors.RadioError
	suite.ErrorAs(err, &radioErr)
	suite.Equal(apperrors.CodeRadioLifecycleInvalid, radioErr.Code)

	// No partial write: name and lifecycle_state are unchanged.
	reloaded, getErr := suite.radioService.GetShow(show.ID)
	suite.Require().NoError(getErr)
	suite.Equal("Morning Show", reloaded.Name, "name must not be persisted when validation fails")
	suite.Equal(catalogm.RadioLifecycleActive, reloaded.LifecycleState)
}

func (suite *RadioServiceIntegrationTestSuite) TestDeleteShow_Success() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")

	err := suite.radioService.DeleteShow(show.ID)

	suite.NoError(err)
	_, err = suite.radioService.GetShow(show.ID)
	suite.Error(err)
}

func (suite *RadioServiceIntegrationTestSuite) TestDeleteShow_NotFound() {
	err := suite.radioService.DeleteShow(99999)

	suite.Error(err)
}

// =============================================================================
// EPISODE TESTS
// =============================================================================

func (suite *RadioServiceIntegrationTestSuite) TestGetEpisodes_Paginated() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	suite.createEpisode(show.ID, "2026-01-01")
	suite.createEpisode(show.ID, "2026-01-02")
	suite.createEpisode(show.ID, "2026-01-03")

	// First page
	episodes, total, err := suite.radioService.GetEpisodes(show.ID, 2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Len(episodes, 2)
	// Ordered by air_date DESC
	suite.Equal("2026-01-03", episodes[0].AirDate)
	suite.Equal("2026-01-02", episodes[1].AirDate)

	// Second page
	episodes, _, err = suite.radioService.GetEpisodes(show.ID, 2, 2)
	suite.Require().NoError(err)
	suite.Len(episodes, 1)
	suite.Equal("2026-01-01", episodes[0].AirDate)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetEpisodeByShowAndDate_Success() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	suite.createEpisode(show.ID, "2026-01-15")

	resp, err := suite.radioService.GetEpisodeByShowAndDate(show.ID, "2026-01-15")

	suite.Require().NoError(err)
	suite.Equal("2026-01-15", resp.AirDate)
	suite.Equal("Morning Show", resp.ShowName)
	suite.Equal("KEXP", resp.StationName)
	// PSY-1306: windowless episode + timezone-less station expose nils.
	suite.Nil(resp.StartsAt)
	suite.Nil(resp.EndsAt)
	suite.Nil(resp.StationTimezone)
}

// TestGetEpisodeByShowAndDate_WindowAndTimezone pins the PSY-1306 detail
// pass-through: the frozen air window and the station's IANA zone ride along
// so the playlist page can render its "aired ..." line viewer-local with a
// station-local aside.
func (suite *RadioServiceIntegrationTestSuite) TestGetEpisodeByShowAndDate_WindowAndTimezone() {
	station := suite.createStation("WFMU Detail")
	suite.Require().NoError(suite.db.Model(&catalogm.RadioStation{}).
		Where("id = ?", station.ID).Update("timezone", "America/New_York").Error)
	show := suite.createShow(station.ID, "Evening Show")
	now := time.Now().UTC()
	starts := now.Add(-6 * time.Hour)
	ends := now.Add(-3 * time.Hour)
	airDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	suite.createEpisodeWindowed(show.ID, airDate, &starts, &ends, 5)

	resp, err := suite.radioService.GetEpisodeByShowAndDate(show.ID, airDate)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp.StartsAt)
	suite.True(resp.StartsAt.Equal(starts))
	suite.Require().NotNil(resp.EndsAt)
	suite.True(resp.EndsAt.Equal(ends))
	suite.Require().NotNil(resp.StationTimezone)
	suite.Equal("America/New_York", *resp.StationTimezone)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetEpisodeByShowAndDate_NotFound() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")

	_, err := suite.radioService.GetEpisodeByShowAndDate(show.ID, "2099-12-31")

	suite.Error(err)
	var radioErr *apperrors.RadioError
	suite.ErrorAs(err, &radioErr)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetEpisodeDetail_WithPlays() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	artist := suite.createArtist("Radiohead")
	suite.createPlay(ep.ID, 0, "Radiohead")
	// Play linked to our artist
	linkedPlay := &catalogm.RadioPlay{
		EpisodeID:  ep.ID,
		Position:   1,
		ArtistName: "Radiohead",
		ArtistID:   &artist.ID,
	}
	suite.Require().NoError(suite.db.Create(linkedPlay).Error)

	resp, err := suite.radioService.GetEpisodeDetail(ep.ID)

	suite.Require().NoError(err)
	suite.Equal("2026-01-15", resp.AirDate)
	suite.Len(resp.Plays, 2)
	// Ordered by position ASC
	suite.Equal(0, resp.Plays[0].Position)
	suite.Nil(resp.Plays[0].ArtistID)
	suite.Equal(1, resp.Plays[1].Position)
	suite.NotNil(resp.Plays[1].ArtistID)
	suite.Equal(artist.ID, *resp.Plays[1].ArtistID)
	suite.NotNil(resp.Plays[1].ArtistSlug)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetEpisodeDetail_NotFound() {
	_, err := suite.radioService.GetEpisodeDetail(99999)

	suite.Error(err)
	var radioErr *apperrors.RadioError
	suite.ErrorAs(err, &radioErr)
}

// =============================================================================
// TOP ARTISTS / LABELS TESTS
// =============================================================================

func (suite *RadioServiceIntegrationTestSuite) TestGetTopArtistsForShow_Success() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep1 := suite.createEpisode(show.ID, "2026-01-01")
	ep2 := suite.createEpisode(show.ID, "2026-01-02")

	// Radiohead played 3 times, Deerhunter 1 time
	suite.createPlay(ep1.ID, 0, "Radiohead")
	suite.createPlay(ep1.ID, 1, "Radiohead")
	suite.createPlay(ep2.ID, 0, "Radiohead")
	suite.createPlay(ep2.ID, 1, "Deerhunter")

	resp, err := suite.radioService.GetTopArtistsForShow(show.ID, 0, 10)

	suite.Require().NoError(err)
	suite.Len(resp, 2)
	suite.Equal("Radiohead", resp[0].ArtistName)
	suite.Equal(3, resp[0].PlayCount)
	suite.Equal(2, resp[0].EpisodeCount)
	suite.Equal("Deerhunter", resp[1].ArtistName)
	suite.Equal(1, resp[1].PlayCount)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetTopArtistsForShow_WithPeriod() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")

	// Recent episode
	recentDate := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	ep1 := suite.createEpisode(show.ID, recentDate)
	suite.createPlay(ep1.ID, 0, "NewBand")

	// Old episode (>90 days)
	oldDate := time.Now().AddDate(0, 0, -100).Format("2006-01-02")
	ep2 := suite.createEpisode(show.ID, oldDate)
	suite.createPlay(ep2.ID, 0, "OldBand")

	resp, err := suite.radioService.GetTopArtistsForShow(show.ID, 90, 10)

	suite.Require().NoError(err)
	suite.Len(resp, 1)
	suite.Equal("NewBand", resp[0].ArtistName)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetTopLabelsForShow_Success() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-01")

	labelName := "Sub Pop"
	play1 := &catalogm.RadioPlay{EpisodeID: ep.ID, Position: 0, ArtistName: "A", LabelName: &labelName}
	play2 := &catalogm.RadioPlay{EpisodeID: ep.ID, Position: 1, ArtistName: "B", LabelName: &labelName}
	suite.Require().NoError(suite.db.Create(play1).Error)
	suite.Require().NoError(suite.db.Create(play2).Error)

	resp, err := suite.radioService.GetTopLabelsForShow(show.ID, 0, 10)

	suite.Require().NoError(err)
	suite.Len(resp, 1)
	suite.Equal("Sub Pop", resp[0].LabelName)
	suite.Equal(2, resp[0].PlayCount)
}

// =============================================================================
// AS HEARD ON TESTS
// =============================================================================

func (suite *RadioServiceIntegrationTestSuite) TestGetAsHeardOnForArtist_Success() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-01")

	artist := suite.createArtist("Radiohead")
	play := &catalogm.RadioPlay{
		EpisodeID:  ep.ID,
		Position:   0,
		ArtistName: "Radiohead",
		ArtistID:   &artist.ID,
	}
	suite.Require().NoError(suite.db.Create(play).Error)

	resp, err := suite.radioService.GetAsHeardOnForArtist(artist.ID)

	suite.Require().NoError(err)
	suite.Len(resp, 1)
	suite.Equal("KEXP", resp[0].StationName)
	suite.Equal("Morning Show", resp[0].ShowName)
	suite.Equal(1, resp[0].PlayCount)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetAsHeardOnForArtist_NoResults() {
	artist := suite.createArtist("Nobody")

	resp, err := suite.radioService.GetAsHeardOnForArtist(artist.ID)

	suite.Require().NoError(err)
	suite.Empty(resp)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetAsHeardOnForRelease_Success() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-01")

	release := suite.createRelease("OK Computer")
	play := &catalogm.RadioPlay{
		EpisodeID:  ep.ID,
		Position:   0,
		ArtistName: "Radiohead",
		ReleaseID:  &release.ID,
	}
	suite.Require().NoError(suite.db.Create(play).Error)

	resp, err := suite.radioService.GetAsHeardOnForRelease(release.ID)

	suite.Require().NoError(err)
	suite.Len(resp, 1)
	suite.Equal("KEXP", resp[0].StationName)
}

// =============================================================================
// NEW RELEASE RADAR TESTS
// =============================================================================

func (suite *RadioServiceIntegrationTestSuite) TestGetNewReleaseRadar_SingleStation() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-01")

	album := "Moon Shaped Pool"
	play := &catalogm.RadioPlay{
		EpisodeID:  ep.ID,
		Position:   0,
		ArtistName: "Radiohead",
		AlbumTitle: &album,
		IsNew:      true,
	}
	suite.Require().NoError(suite.db.Create(play).Error)

	resp, err := suite.radioService.GetNewReleaseRadar(station.ID, 10)

	suite.Require().NoError(err)
	suite.Len(resp, 1)
	suite.Equal("Radiohead", resp[0].ArtistName)
	suite.Equal(&album, resp[0].AlbumTitle)
	suite.Equal(1, resp[0].StationCount)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetNewReleaseRadar_CrossStation_RequiresTwo() {
	station1 := suite.createStation("KEXP")
	show1 := suite.createShow(station1.ID, "KEXP Morning")
	ep1 := suite.createEpisode(show1.ID, "2026-01-01")

	album := "Moon Shaped Pool"
	play1 := &catalogm.RadioPlay{EpisodeID: ep1.ID, Position: 0, ArtistName: "Radiohead", AlbumTitle: &album, IsNew: true}
	suite.Require().NoError(suite.db.Create(play1).Error)

	// Only one station, cross-station query should return nothing
	resp, err := suite.radioService.GetNewReleaseRadar(0, 10)
	suite.Require().NoError(err)
	suite.Empty(resp)

	// Add second station playing same new release
	station2 := suite.createStation("WFMU")
	show2 := suite.createShow(station2.ID, "WFMU Show")
	ep2 := suite.createEpisode(show2.ID, "2026-01-02")
	play2 := &catalogm.RadioPlay{EpisodeID: ep2.ID, Position: 0, ArtistName: "Radiohead", AlbumTitle: &album, IsNew: true}
	suite.Require().NoError(suite.db.Create(play2).Error)

	resp, err = suite.radioService.GetNewReleaseRadar(0, 10)
	suite.Require().NoError(err)
	suite.Len(resp, 1)
	suite.Equal(2, resp[0].StationCount)
}

// =============================================================================
// STATS TESTS
// =============================================================================

func (suite *RadioServiceIntegrationTestSuite) TestGetRadioStats_Empty() {
	resp, err := suite.radioService.GetRadioStats()

	suite.Require().NoError(err)
	suite.Equal(0, resp.TotalStations)
	suite.Equal(0, resp.TotalShows)
	suite.Equal(0, resp.TotalEpisodes)
	suite.Equal(int64(0), resp.TotalPlays)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetRadioStats_WithData() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-01")

	artist := suite.createArtist("Radiohead")
	linked := &catalogm.RadioPlay{EpisodeID: ep.ID, Position: 0, ArtistName: "Radiohead", ArtistID: &artist.ID}
	unlinked := &catalogm.RadioPlay{EpisodeID: ep.ID, Position: 1, ArtistName: "Unknown"}
	suite.Require().NoError(suite.db.Create(linked).Error)
	suite.Require().NoError(suite.db.Create(unlinked).Error)

	resp, err := suite.radioService.GetRadioStats()

	suite.Require().NoError(err)
	suite.Equal(1, resp.TotalStations)
	suite.Equal(1, resp.TotalShows)
	suite.Equal(1, resp.TotalEpisodes)
	suite.Equal(int64(2), resp.TotalPlays)
	suite.Equal(int64(1), resp.MatchedPlays)
	suite.Equal(1, resp.UniqueArtists)
}

// =============================================================================
// CASCADE DELETE TESTS
// =============================================================================

func (suite *RadioServiceIntegrationTestSuite) TestDeleteStation_CascadesShows() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-01")
	suite.createPlay(ep.ID, 0, "Radiohead")

	err := suite.radioService.DeleteStation(station.ID)
	suite.Require().NoError(err)

	// Verify cascade — shows, episodes, and plays should all be gone
	var showCount int64
	suite.db.Model(&catalogm.RadioShow{}).Where("station_id = ?", station.ID).Count(&showCount)
	suite.Equal(int64(0), showCount)

	var epCount int64
	suite.db.Model(&catalogm.RadioEpisode{}).Where("show_id = ?", show.ID).Count(&epCount)
	suite.Equal(int64(0), epCount)

	var playCount int64
	suite.db.Model(&catalogm.RadioPlay{}).Where("episode_id = ?", ep.ID).Count(&playCount)
	suite.Equal(int64(0), playCount)
}

// =============================================================================
// PSY-508: RADIO_NETWORKS RELATIONSHIP TESTS
// =============================================================================

// TestRadioNetwork_WFMUSeed_FourStationsUnderNetwork verifies the migration
// shipped with PSY-508 wires up the WFMU network and its 4 sibling stations
// (the 91.1 broadcast plus three stream-only sub-channels).
//
// This is a guard against migration regressions: if someone breaks the
// network seed or the network_id backfill, this test fails fast.
func (suite *RadioServiceIntegrationTestSuite) TestRadioNetwork_WFMUSeed_FourStationsUnderNetwork() {
	// The migration ships the network row + 4 stations. Re-apply it
	// inside the test scope (TearDownTest wipes everything between
	// tests in this suite, so we re-seed for this test).
	suite.Require().NoError(suite.db.Exec(`
		INSERT INTO radio_networks (slug, name) VALUES ('wfmu', 'WFMU')
	`).Error)
	suite.Require().NoError(suite.db.Exec(`
		INSERT INTO radio_stations (name, slug, broadcast_type, playlist_source, network_id, is_active)
		VALUES
		    ('WFMU', 'wfmu', 'both', 'wfmu_scrape', (SELECT id FROM radio_networks WHERE slug = 'wfmu'), TRUE),
		    ('Give the Drummer Radio', 'wfmu-drummer', 'internet', 'wfmu_scrape', (SELECT id FROM radio_networks WHERE slug = 'wfmu'), TRUE),
		    ('Rock''n''Soul Radio', 'wfmu-rocknsoulradio', 'internet', 'wfmu_scrape', (SELECT id FROM radio_networks WHERE slug = 'wfmu'), TRUE),
		    ('Sheena''s Jungle Room', 'wfmu-sheena', 'internet', 'wfmu_scrape', (SELECT id FROM radio_networks WHERE slug = 'wfmu'), TRUE)
	`).Error)

	// Network row exists with the expected slug + name.
	var network catalogm.RadioNetwork
	err := suite.db.Where("slug = ?", "wfmu").First(&network).Error
	suite.Require().NoError(err)
	suite.Equal("wfmu", network.Slug)
	suite.Equal("WFMU", network.Name)

	// Exactly 4 stations point at the WFMU network.
	var stations []catalogm.RadioStation
	err = suite.db.Where("network_id = ?", network.ID).Order("slug ASC").Find(&stations).Error
	suite.Require().NoError(err)
	suite.Require().Len(stations, 4)

	// Slugs match the locked design (alphabetical order from query above).
	suite.Equal("wfmu", stations[0].Slug)
	suite.Equal("wfmu-drummer", stations[1].Slug)
	suite.Equal("wfmu-rocknsoulradio", stations[2].Slug)
	suite.Equal("wfmu-sheena", stations[3].Slug)

	// All four share the same network_id (sibling, not nested).
	for _, st := range stations {
		suite.Require().NotNil(st.NetworkID)
		suite.Equal(network.ID, *st.NetworkID)
	}
}

// TestRadioNetwork_StationDelete_PreservesNetwork verifies that deleting a
// station does not delete the network. ON DELETE SET NULL on the FK only
// applies when the *network* is deleted, not when a station is deleted.
func (suite *RadioServiceIntegrationTestSuite) TestRadioNetwork_StationDelete_PreservesNetwork() {
	// Seed: 1 network, 1 station pointing at it.
	suite.Require().NoError(suite.db.Exec(`INSERT INTO radio_networks (slug, name) VALUES ('test-net', 'Test Net')`).Error)
	var network catalogm.RadioNetwork
	suite.Require().NoError(suite.db.Where("slug = ?", "test-net").First(&network).Error)

	station := &catalogm.RadioStation{
		Name:          "Test Station",
		Slug:          "test-station",
		BroadcastType: "internet",
		NetworkID:     &network.ID,
	}
	suite.Require().NoError(suite.db.Create(station).Error)

	// Delete the station.
	suite.Require().NoError(suite.radioService.DeleteStation(station.ID))

	// Network should still exist.
	var count int64
	suite.db.Model(&catalogm.RadioNetwork{}).Where("slug = ?", "test-net").Count(&count)
	suite.Equal(int64(1), count)
}

// TestRadioNetwork_NetworkDelete_StationsKeptWithNullNetworkID verifies that
// deleting a network sets affiliated stations' network_id to NULL (rather
// than cascading the delete). The schema FK is ON DELETE SET NULL.
func (suite *RadioServiceIntegrationTestSuite) TestRadioNetwork_NetworkDelete_StationsKeptWithNullNetworkID() {
	// Seed: 1 network, 1 station pointing at it.
	suite.Require().NoError(suite.db.Exec(`INSERT INTO radio_networks (slug, name) VALUES ('cleanup-net', 'Cleanup Net')`).Error)
	var network catalogm.RadioNetwork
	suite.Require().NoError(suite.db.Where("slug = ?", "cleanup-net").First(&network).Error)

	station := &catalogm.RadioStation{
		Name:          "Affiliated",
		Slug:          "affiliated-cleanup",
		BroadcastType: "internet",
		NetworkID:     &network.ID,
	}
	suite.Require().NoError(suite.db.Create(station).Error)

	// Delete the network.
	suite.Require().NoError(suite.db.Delete(&network).Error)

	// Station should still exist with network_id = NULL.
	var refreshed catalogm.RadioStation
	suite.Require().NoError(suite.db.First(&refreshed, station.ID).Error)
	suite.Nil(refreshed.NetworkID)
}

// TestRadioNetwork_GetStationBySlug_PreloadsNetworkSlug verifies that the
// public station detail response includes the network_slug (via Network
// preload) so clients can identify a station's network without a second
// round-trip.
func (suite *RadioServiceIntegrationTestSuite) TestRadioNetwork_GetStationBySlug_PreloadsNetworkSlug() {
	suite.Require().NoError(suite.db.Exec(`INSERT INTO radio_networks (slug, name) VALUES ('wfmu', 'WFMU')`).Error)
	var network catalogm.RadioNetwork
	suite.Require().NoError(suite.db.Where("slug = ?", "wfmu").First(&network).Error)

	station := &catalogm.RadioStation{
		Name:          "Give the Drummer Radio",
		Slug:          "wfmu-drummer",
		BroadcastType: "internet",
		NetworkID:     &network.ID,
	}
	suite.Require().NoError(suite.db.Create(station).Error)

	resp, err := suite.radioService.GetStationBySlug("wfmu-drummer")
	suite.Require().NoError(err)
	suite.Require().NotNil(resp.NetworkID)
	suite.Equal(network.ID, *resp.NetworkID)
	suite.Require().NotNil(resp.NetworkSlug)
	suite.Equal("wfmu", *resp.NetworkSlug)
}

// =============================================================================
// PSY-669: NETWORK INFO + SIBLING_STATIONS PROJECTION TESTS
// =============================================================================

// seedWFMUNetwork seeds the canonical WFMU network + 4 stations (flagship +
// 3 stream-only sub-channels) used by the PSY-669 network projection tests.
// Returns the seeded stations by slug for easy lookup.
func (suite *RadioServiceIntegrationTestSuite) seedWFMUNetwork() (network catalogm.RadioNetwork, stationsBySlug map[string]catalogm.RadioStation) {
	suite.Require().NoError(suite.db.Exec(`INSERT INTO radio_networks (slug, name) VALUES ('wfmu', 'WFMU')`).Error)
	suite.Require().NoError(suite.db.Where("slug = ?", "wfmu").First(&network).Error)

	flagshipFreq := 91.1
	rows := []catalogm.RadioStation{
		{Name: "WFMU", Slug: "wfmu", BroadcastType: catalogm.BroadcastTypeBoth, FrequencyMHz: &flagshipFreq, IsActive: true, NetworkID: &network.ID, IsFlagship: true},
		{Name: "Give the Drummer Radio", Slug: "wfmu-drummer", BroadcastType: catalogm.BroadcastTypeInternet, IsActive: true, NetworkID: &network.ID, IsFlagship: false},
		{Name: "Rock'n'Soul Radio", Slug: "wfmu-rocknsoulradio", BroadcastType: catalogm.BroadcastTypeInternet, IsActive: true, NetworkID: &network.ID, IsFlagship: false},
		{Name: "Sheena's Jungle Room", Slug: "wfmu-sheena", BroadcastType: catalogm.BroadcastTypeInternet, IsActive: true, NetworkID: &network.ID, IsFlagship: false},
	}
	stationsBySlug = make(map[string]catalogm.RadioStation, len(rows))
	for i := range rows {
		suite.Require().NoError(suite.db.Create(&rows[i]).Error)
		stationsBySlug[rows[i].Slug] = rows[i]
	}
	return network, stationsBySlug
}

// TestRadioNetwork_GetStation_FlagshipResponse verifies the flagship station's
// detail response carries Network.IsFlagship=true and SiblingStations contains
// the 3 non-flagship sub-streams (self excluded), sorted alphabetically since
// they share the same is_flagship=false level.
func (suite *RadioServiceIntegrationTestSuite) TestRadioNetwork_GetStation_FlagshipResponse() {
	_, stations := suite.seedWFMUNetwork()
	flagship := stations["wfmu"]

	resp, err := suite.radioService.GetStation(flagship.ID)
	suite.Require().NoError(err)

	suite.Require().NotNil(resp.Network)
	suite.Equal("wfmu", resp.Network.Slug)
	suite.Equal("WFMU", resp.Network.Name)
	suite.True(resp.Network.IsFlagship, "flagship station should report is_flagship=true on its network info")

	suite.Require().Len(resp.SiblingStations, 3, "flagship should see 3 sibling sub-streams")
	suite.Equal("Give the Drummer Radio", resp.SiblingStations[0].Name)
	suite.Equal("Rock'n'Soul Radio", resp.SiblingStations[1].Name)
	suite.Equal("Sheena's Jungle Room", resp.SiblingStations[2].Name)
	for _, sib := range resp.SiblingStations {
		suite.False(sib.IsFlagship, "siblings of the flagship should all be non-flagship: got %s flagged", sib.Slug)
		suite.NotEqual(flagship.ID, sib.ID, "self must be excluded from siblings")
	}
}

// TestRadioNetwork_GetStation_SiblingResponse verifies that fetching a
// non-flagship sub-stream returns Network.IsFlagship=false and SiblingStations
// includes the flagship FIRST (flagship-first ordering matters for the tab bar
// UI which leads with the main station).
func (suite *RadioServiceIntegrationTestSuite) TestRadioNetwork_GetStation_SiblingResponse() {
	_, stations := suite.seedWFMUNetwork()
	drummer := stations["wfmu-drummer"]

	resp, err := suite.radioService.GetStation(drummer.ID)
	suite.Require().NoError(err)

	suite.Require().NotNil(resp.Network)
	suite.Equal("wfmu", resp.Network.Slug)
	suite.False(resp.Network.IsFlagship, "drummer is not the flagship")

	suite.Require().Len(resp.SiblingStations, 3, "drummer should see 3 siblings: flagship + 2 other sub-streams")
	suite.Equal("WFMU", resp.SiblingStations[0].Name, "flagship must come first in sibling ordering")
	suite.True(resp.SiblingStations[0].IsFlagship)
	// PSY-676: flagship-as-sibling carries frequency so NetworkTabBar can render
	// the flagship tab as "WFMU 91.1" from any sub-stream page, matching the
	// label that renders from the flagship's own /radio/wfmu page.
	suite.Require().NotNil(resp.SiblingStations[0].FrequencyMHz, "flagship sibling should carry frequency_mhz")
	suite.InDelta(91.1, *resp.SiblingStations[0].FrequencyMHz, 0.001)
	suite.Equal("Rock'n'Soul Radio", resp.SiblingStations[1].Name)
	suite.False(resp.SiblingStations[1].IsFlagship)
	suite.Nil(resp.SiblingStations[1].FrequencyMHz, "internet-only sub-stream has no frequency")
	suite.Equal("Sheena's Jungle Room", resp.SiblingStations[2].Name)
	suite.False(resp.SiblingStations[2].IsFlagship)

	for _, sib := range resp.SiblingStations {
		suite.NotEqual(drummer.ID, sib.ID, "self must be excluded from siblings")
	}
}

// TestRadioNetwork_GetStation_NetworkLessResponse verifies a station that
// belongs to no network returns Network=nil and SiblingStations as an empty
// (non-nil) slice. Frontend depends on the JSON shape being a stable `[]`,
// not `null`, so iterators don't trip.
func (suite *RadioServiceIntegrationTestSuite) TestRadioNetwork_GetStation_NetworkLessResponse() {
	station := &catalogm.RadioStation{
		Name:          "KEXP",
		Slug:          "kexp",
		BroadcastType: catalogm.BroadcastTypeBoth,
		IsActive:      true,
	}
	suite.Require().NoError(suite.db.Create(station).Error)

	resp, err := suite.radioService.GetStation(station.ID)
	suite.Require().NoError(err)

	suite.Nil(resp.Network)
	suite.NotNil(resp.SiblingStations, "JSON shape must be `[]`, not `null`")
	suite.Empty(resp.SiblingStations)
}

// TestRadioNetwork_GetStationBySlug_PopulatesNetworkAndSiblings is the
// slug-keyed counterpart to TestRadioNetwork_GetStation_SiblingResponse and
// guards against regressions where one of the two single-station fetch paths
// drifts away from the other.
func (suite *RadioServiceIntegrationTestSuite) TestRadioNetwork_GetStationBySlug_PopulatesNetworkAndSiblings() {
	suite.seedWFMUNetwork()

	resp, err := suite.radioService.GetStationBySlug("wfmu-sheena")
	suite.Require().NoError(err)

	suite.Require().NotNil(resp.Network)
	suite.Equal("wfmu", resp.Network.Slug)
	suite.False(resp.Network.IsFlagship)
	suite.Require().Len(resp.SiblingStations, 3)
	suite.Equal("WFMU", resp.SiblingStations[0].Name, "flagship must come first")
	suite.True(resp.SiblingStations[0].IsFlagship)
	suite.Require().NotNil(resp.SiblingStations[0].FrequencyMHz, "slug-keyed path must also carry flagship frequency_mhz")
	suite.InDelta(91.1, *resp.SiblingStations[0].FrequencyMHz, 0.001)
}

// TestRadioNetwork_ListStations_PopulatesNetworkAndSiblings verifies the
// batch path used by GET /radio-stations. Asserts every station in the
// response gets correctly-populated Network info + siblings, AND that
// network-less stations in the same response are unaffected.
func (suite *RadioServiceIntegrationTestSuite) TestRadioNetwork_ListStations_PopulatesNetworkAndSiblings() {
	suite.seedWFMUNetwork()
	// Add a network-less station to the same response to exercise the
	// mixed-case branch (some rows have a network, some don't).
	kexp := &catalogm.RadioStation{
		Name: "KEXP", Slug: "kexp", BroadcastType: catalogm.BroadcastTypeBoth, IsActive: true,
	}
	suite.Require().NoError(suite.db.Create(kexp).Error)

	resp, err := suite.radioService.ListStations(map[string]interface{}{"is_active": true})
	suite.Require().NoError(err)
	suite.Require().Len(resp, 5, "expected 4 WFMU stations + 1 network-less KEXP")

	bySlug := map[string]*contracts.RadioStationListResponse{}
	for _, r := range resp {
		bySlug[r.Slug] = r
	}

	// Flagship sees 3 non-flagship siblings.
	wfmu := bySlug["wfmu"]
	suite.Require().NotNil(wfmu)
	suite.Require().NotNil(wfmu.Network)
	suite.True(wfmu.Network.IsFlagship)
	suite.Require().Len(wfmu.SiblingStations, 3)
	for _, sib := range wfmu.SiblingStations {
		suite.False(sib.IsFlagship)
	}

	// A sub-stream sees flagship + 2 other sub-streams.
	drummer := bySlug["wfmu-drummer"]
	suite.Require().NotNil(drummer)
	suite.Require().NotNil(drummer.Network)
	suite.False(drummer.Network.IsFlagship)
	suite.Require().Len(drummer.SiblingStations, 3)
	suite.Equal("WFMU", drummer.SiblingStations[0].Name, "flagship-first ordering")
	suite.True(drummer.SiblingStations[0].IsFlagship)
	suite.Require().NotNil(drummer.SiblingStations[0].FrequencyMHz, "list path must also carry flagship frequency_mhz")
	suite.InDelta(91.1, *drummer.SiblingStations[0].FrequencyMHz, 0.001)

	// Network-less station has nil Network and empty SiblingStations.
	k := bySlug["kexp"]
	suite.Require().NotNil(k)
	suite.Nil(k.Network)
	suite.NotNil(k.SiblingStations)
	suite.Empty(k.SiblingStations)
}

// =============================================================================
// LATEST-PLAYLISTS FEEDS + STATION AGGREGATIONS (PSY-1048)
// =============================================================================

func TestRadioService_NilDB_Feeds(t *testing.T) {
	svc := &RadioService{db: nil}
	assertNilDBError(t, func() error { _, _, err := svc.GetStationEpisodes(1, 10, 0); return err })
	assertNilDBError(t, func() error { _, _, err := svc.GetRecentEpisodes(10, 0); return err })
	assertNilDBError(t, func() error { _, err := svc.GetTopArtistsForStation(1, 90, 10); return err })
	assertNilDBError(t, func() error { _, err := svc.GetTopLabelsForStation(1, 90, 10); return err })
}

// createNetworkFamily seeds a network with a flagship + one sibling channel,
// plus one standalone station outside the network. Returns (flagship,
// sibling, standalone).
func (suite *RadioServiceIntegrationTestSuite) createNetworkFamily() (*catalogm.RadioStation, *catalogm.RadioStation, *catalogm.RadioStation) {
	suite.Require().NoError(suite.db.Exec(`INSERT INTO radio_networks (slug, name) VALUES ('fam-net', 'Family Net')`).Error)
	var network catalogm.RadioNetwork
	suite.Require().NoError(suite.db.Where("slug = ?", "fam-net").First(&network).Error)

	flagship := &catalogm.RadioStation{Name: "Flagship", Slug: "flagship", BroadcastType: "both", NetworkID: &network.ID, IsFlagship: true}
	suite.Require().NoError(suite.db.Create(flagship).Error)
	sibling := &catalogm.RadioStation{Name: "Channel Two", Slug: "channel-two", BroadcastType: "internet", NetworkID: &network.ID}
	suite.Require().NoError(suite.db.Create(sibling).Error)
	standalone := &catalogm.RadioStation{Name: "Standalone", Slug: "standalone", BroadcastType: "both"}
	suite.Require().NoError(suite.db.Create(standalone).Error)
	return flagship, sibling, standalone
}

func (suite *RadioServiceIntegrationTestSuite) TestListShows_LatestAirDateAndSort() {
	// Relative dates: the latest-playlist badge is now aired-only-bounded
	// (PSY-1205), so fixed dates would couple this to the wall clock. All dates
	// are in the past so they stay aired; ordering is what's asserted.
	now := time.Now().UTC()
	alphaLatest := now.AddDate(0, 0, -18).Format("2006-01-02")

	station := suite.createStation("KSRT")
	older := suite.createShow(station.ID, "Alpha Show")
	suite.createAiredEpisode(older.ID, now.AddDate(0, 0, -22).Format("2006-01-02"))
	suite.createAiredEpisode(older.ID, alphaLatest)
	fresh := suite.createShow(station.ID, "Zulu Show")
	suite.createAiredEpisode(fresh.ID, now.AddDate(0, 0, -14).Format("2006-01-02")) // newest
	suite.createShow(station.ID, "Mid Show")
	retired := suite.createShow(station.ID, "Beta Retired")
	suite.createAiredEpisode(retired.ID, now.AddDate(0, 0, -16).Format("2006-01-02"))
	suite.Require().NoError(suite.db.Model(&catalogm.RadioShow{}).Where("id = ?", retired.ID).Update("is_active", false).Error)
	suite.Require().NoError(suite.db.Model(&catalogm.RadioShow{}).Where("id = ?", older.ID).Update("schedule_display", "Mon 9pm-12am").Error)

	// Default sort stays alphabetical (existing behavior).
	byName, err := suite.radioService.ListShows(station.ID, "")
	suite.Require().NoError(err)
	suite.Require().Len(byName, 4)
	suite.Equal("Alpha Show", byName[0].Name)
	suite.Require().NotNil(byName[0].LatestAirDate)
	suite.Equal(alphaLatest, *byName[0].LatestAirDate)
	// schedule_display rides along on list rows (PSY-1050 shows directory).
	suite.Require().NotNil(byName[0].ScheduleDisplay)
	suite.Equal("Mon 9pm-12am", *byName[0].ScheduleDisplay)
	suite.Nil(byName[1].ScheduleDisplay)

	// sort=latest: active shows first, newest playlist first, episode-less
	// actives after dated actives, inactive shows last.
	byLatest, err := suite.radioService.ListShows(station.ID, RadioShowSortLatest)
	suite.Require().NoError(err)
	suite.Require().Len(byLatest, 4)
	suite.Equal("Zulu Show", byLatest[0].Name)
	suite.Equal("Alpha Show", byLatest[1].Name)
	suite.Equal("Mid Show", byLatest[2].Name)
	suite.Nil(byLatest[2].LatestAirDate)
	suite.Equal("Beta Retired", byLatest[3].Name)
	suite.False(byLatest[3].IsActive)
}

// TestListShows_LatestEpisodeWindow pins the PSY-1306 LAST-column window: each
// show row carries the frozen air window of the SAME episode LatestAirDate
// names (the feed's latest-first winner), not MAX(starts_at) — the fixture
// gives an OLDER episode the newer starts_at so a naive MAX would fail. A
// windowless latest yields a nil window with the date still set; no episodes
// yields nil everything.
func (suite *RadioServiceIntegrationTestSuite) TestListShows_LatestEpisodeWindow() {
	now := time.Now().UTC()
	station := suite.createStation("Window FM")

	windowed := suite.createShow(station.ID, "Windowed Show")
	// older episode deliberately carries the NEWEST starts_at…
	oldStarts := now.Add(-5 * time.Hour)
	oldEnds := now.Add(-4 * time.Hour)
	suite.createEpisodeWindowed(windowed.ID, now.AddDate(0, 0, -10).Format("2006-01-02"), &oldStarts, &oldEnds, 9)
	// …while the latest-by-date episode has an older window: DISTINCT ON must
	// return THIS one.
	latestStarts := now.Add(-80 * time.Hour)
	latestEnds := now.Add(-77 * time.Hour)
	latestDate := now.AddDate(0, 0, -2).Format("2006-01-02")
	suite.createEpisodeWindowed(windowed.ID, latestDate, &latestStarts, &latestEnds, 12)

	popup := suite.createShow(station.ID, "Popup Show")
	suite.createEpisodeWindowed(popup.ID, now.AddDate(0, 0, -3).Format("2006-01-02"), nil, nil, 4)

	empty := suite.createShow(station.ID, "Empty Show")

	rows, err := suite.radioService.ListShows(station.ID, "")
	suite.Require().NoError(err)
	byID := map[uint]*contracts.RadioShowListResponse{}
	for _, r := range rows {
		byID[r.ID] = r
	}

	w := byID[windowed.ID]
	suite.Require().NotNil(w.LatestAirDate)
	suite.Equal(latestDate, *w.LatestAirDate)
	suite.Require().NotNil(w.LatestStartsAt, "window must come from the latest-by-date episode")
	suite.True(w.LatestStartsAt.Equal(latestStarts), "must be the latest episode's window, not MAX(starts_at)")
	suite.Require().NotNil(w.LatestEndsAt)
	suite.True(w.LatestEndsAt.Equal(latestEnds))

	p := byID[popup.ID]
	suite.Require().NotNil(p.LatestAirDate)
	suite.Nil(p.LatestStartsAt, "windowless latest exposes a nil window")
	suite.Nil(p.LatestEndsAt)

	e := byID[empty.ID]
	suite.Nil(e.LatestAirDate)
	suite.Nil(e.LatestStartsAt)
	suite.Nil(e.LatestEndsAt)
}

// TestListShows_LatestAirDateExcludesFuture pins the aired-only "latest playlist"
// badge/sort (PSY-1205): a future-dated placeholder (upcoming WFMU broadcast or a
// corrupt date) must NOT become a show's latest date and sort it to the top of
// the "sorted by latest playlist" directory. tz is pinned to UTC for a
// deterministic day boundary (±2-day margin); see the PSY-1204 future-feed tests.
func (suite *RadioServiceIntegrationTestSuite) TestListShows_LatestAirDateExcludesFuture() {
	station := suite.createStation("KFUT")
	suite.Require().NoError(suite.db.Model(&catalogm.RadioStation{}).
		Where("id = ?", station.ID).Update("timezone", "UTC").Error)
	now := time.Now().UTC()
	recent := now.AddDate(0, 0, -1).Format("2006-01-02")
	stale := now.AddDate(0, 0, -10).Format("2006-01-02")
	future := now.AddDate(0, 0, 5).Format("2006-01-02")

	// Recent Show: latest aired = recent (yesterday).
	recentShow := suite.createShow(station.ID, "Recent Show")
	suite.createAiredEpisode(recentShow.ID, recent)
	// Stale Plus Future: an OLD aired episode + a far-future placeholder. Its
	// latest AIRED date is `stale`; the old buggy MAX(air_date) would read
	// `future` and sort this show ABOVE Recent Show — the precise sort-inversion
	// this fix prevents.
	stalePlusFuture := suite.createShow(station.ID, "Stale Plus Future")
	suite.createAiredEpisode(stalePlusFuture.ID, stale)
	suite.createEpisode(stalePlusFuture.ID, future)
	// Future Only: only a future episode → no aired latest date at all.
	futureOnly := suite.createShow(station.ID, "Future Only")
	suite.createEpisode(futureOnly.ID, future)

	shows, err := suite.radioService.ListShows(station.ID, RadioShowSortLatest)
	suite.Require().NoError(err)
	suite.Require().Len(shows, 3)

	// SORT: Recent (recent) > Stale Plus Future (stale, NOT future) > Future Only
	// (NULL latest). The future placeholder must not lift Stale Plus Future above
	// the genuinely-more-recent Recent Show.
	suite.Equal("Recent Show", shows[0].Name)
	suite.Equal("Stale Plus Future", shows[1].Name)
	suite.Equal("Future Only", shows[2].Name)

	// VALUE: the badge reads the aired date, not the future one; COUNT still
	// includes the future placeholder (only the latest-date is bounded).
	suite.Require().NotNil(shows[1].LatestAirDate)
	suite.Equal(stale, *shows[1].LatestAirDate, "latest playlist is the aired episode, not the future placeholder")
	suite.Equal(int64(2), shows[1].EpisodeCount, "COUNT still includes the future placeholder; only the latest-date is bounded")
	suite.Nil(shows[2].LatestAirDate, "a show with only future episodes has no latest playlist")
}

// TestListShows_LatestAirDateExcludesNotYetAired extends the aired-only latest-date
// (PSY-1205) with PSY-1285's air-window precision: a TODAY-dated episode whose frozen
// window is still in the future (a WFMU broadcast airing later today) must not become
// a show's latest playlist date — the day-granular air_date <= today bound can't catch
// a same-day not-yet-aired row, but the air window can.
func (suite *RadioServiceIntegrationTestSuite) TestListShows_LatestAirDateExcludesNotYetAired() {
	station := suite.createStation("KLATE")
	suite.Require().NoError(suite.db.Model(&catalogm.RadioStation{}).
		Where("id = ?", station.ID).Update("timezone", "UTC").Error)
	now := time.Now().UTC()
	ptr := func(t time.Time) *time.Time { return &t }
	aired := now.AddDate(0, 0, -1).Format("2006-01-02")
	today := now.Format("2006-01-02")

	show := suite.createShow(station.ID, "Airs Later Today")
	suite.createEpisodeWindowed(show.ID, aired, ptr(now.Add(-26*time.Hour)), ptr(now.Add(-24*time.Hour)), 0)
	suite.createEpisodeWindowed(show.ID, today, ptr(now.Add(2*time.Hour)), ptr(now.Add(4*time.Hour)), 0)

	shows, err := suite.radioService.ListShows(station.ID, RadioShowSortLatest)
	suite.Require().NoError(err)
	suite.Require().Len(shows, 1)
	suite.Require().NotNil(shows[0].LatestAirDate)
	suite.Equal(aired, *shows[0].LatestAirDate, "latest is yesterday's aired episode, not today's not-yet-aired broadcast")
	suite.Equal(int64(2), shows[0].EpisodeCount, "COUNT still includes the not-yet-aired episode")
}

// TestLatestEpisodeForShow_ExcludesFuture pins the now-playing archive fallback's
// "latest playlist" selection to aired-only (PSY-1205): the station ON-AIR box
// derives its "Latest: {date}" / "aired {date}" text + deep-link from this, and a
// future-dated placeholder would mislabel a not-yet-aired broadcast as aired.
func (suite *RadioServiceIntegrationTestSuite) TestLatestEpisodeForShow_ExcludesFuture() {
	station := suite.createStation("KLATE")
	suite.Require().NoError(suite.db.Model(&catalogm.RadioStation{}).
		Where("id = ?", station.ID).Update("timezone", "UTC").Error)
	show := suite.createShow(station.ID, "Latest Show")
	now := time.Now().UTC()
	aired := now.AddDate(0, 0, -1).Format("2006-01-02")
	future := now.AddDate(0, 0, 3).Format("2006-01-02")
	suite.createAiredEpisode(show.ID, aired)
	suite.createEpisode(show.ID, future)

	ep, err := suite.radioService.latestEpisodeForShow(show.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(ep)
	suite.Equal(aired, normalizeDate(ep.AirDate), "latest is the aired episode, not the future placeholder")
}

func (suite *RadioServiceIntegrationTestSuite) TestGetEpisodes_ArtistPreview() {
	station := suite.createStation("KPRV")
	show := suite.createShow(station.ID, "Preview Show")
	ep := suite.createEpisode(show.ID, "2026-06-09")

	matched := suite.createArtist("Matched Artist")
	first := suite.createArtist("First Artist")
	plays := []catalogm.RadioPlay{
		// "First Artist" plays unmatched at position 1 and matched at 3 —
		// the preview must keep the knowledge-graph link (MAX(artist_id)).
		{EpisodeID: ep.ID, Position: 1, ArtistName: "First Artist"},
		{EpisodeID: ep.ID, Position: 2, ArtistName: "Matched Artist", ArtistID: &matched.ID},
		{EpisodeID: ep.ID, Position: 3, ArtistName: "First Artist", ArtistID: &first.ID},
		{EpisodeID: ep.ID, Position: 4, ArtistName: "Third Artist"},
		{EpisodeID: ep.ID, Position: 5, ArtistName: "Fourth Artist"}, // beyond preview cap
	}
	for i := range plays {
		suite.Require().NoError(suite.db.Create(&plays[i]).Error)
	}

	episodes, total, err := suite.radioService.GetEpisodes(show.ID, 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(episodes, 1)

	preview := episodes[0].ArtistPreview
	suite.Require().Len(preview, episodePreviewArtistCount)
	suite.Equal("First Artist", preview[0].ArtistName)
	suite.Require().NotNil(preview[0].ArtistID, "partially-matched artist must keep its graph link")
	suite.Equal(first.ID, *preview[0].ArtistID)
	suite.Equal("Matched Artist", preview[1].ArtistName)
	suite.Require().NotNil(preview[1].ArtistID)
	suite.Equal(matched.ID, *preview[1].ArtistID)
	suite.Require().NotNil(preview[1].ArtistSlug)
	suite.Equal("Third Artist", preview[2].ArtistName)
}

// TestGetEpisodes_ComputedStatusFromWindow verifies the service-mapping seam
// (PSY-1152): GetEpisodes computes episode Status on READ from the frozen window
// and surfaces starts_at/ends_at. A window containing now → live; a past window
// → aired; no window → aired (never falsely live — the PSY-1128 fix at the wire).
func (suite *RadioServiceIntegrationTestSuite) TestGetEpisodes_ComputedStatusFromWindow() {
	station := suite.createStation("KWIN")
	show := suite.createShow(station.ID, "Window Show")

	now := time.Now()
	pastStart, pastEnd := now.Add(-3*time.Hour), now.Add(-2*time.Hour)
	liveStart, liveEnd := now.Add(-1*time.Hour), now.Add(1*time.Hour)

	live := &catalogm.RadioEpisode{ShowID: show.ID, AirDate: "2026-06-03", StartsAt: &liveStart, EndsAt: &liveEnd}
	windowless := &catalogm.RadioEpisode{ShowID: show.ID, AirDate: "2026-06-02"}
	aired := &catalogm.RadioEpisode{ShowID: show.ID, AirDate: "2026-06-01", StartsAt: &pastStart, EndsAt: &pastEnd}
	for _, e := range []*catalogm.RadioEpisode{live, windowless, aired} {
		suite.Require().NoError(suite.db.Create(e).Error)
	}

	episodes, _, err := suite.radioService.GetEpisodes(show.ID, 10, 0)
	suite.Require().NoError(err)
	suite.Require().Len(episodes, 3)

	// Index by air_date so the assertions don't depend on result ordering.
	statusByDate := map[string]string{}
	startsByDate := map[string]*time.Time{}
	for _, e := range episodes {
		statusByDate[e.AirDate] = e.Status
		startsByDate[e.AirDate] = e.StartsAt
	}

	suite.Equal(catalogm.RadioEpisodeStatusLive, statusByDate["2026-06-03"], "now inside the window → live")
	suite.NotNil(startsByDate["2026-06-03"], "the frozen window is surfaced on the response")
	suite.Equal(catalogm.RadioEpisodeStatusAired, statusByDate["2026-06-02"], "no window → aired, never live")
	suite.Nil(startsByDate["2026-06-02"], "windowless episode has a nil starts_at")
	suite.Equal(catalogm.RadioEpisodeStatusAired, statusByDate["2026-06-01"], "past window → aired")
}

// TestGetEpisodes_FlagsUpcoming verifies the per-show archive (PSY-1205): it
// still LISTS upcoming episodes (the "label, don't hide" decision), but flags
// future-dated ones IsUpcoming so the UI can tag them instead of rendering empty
// aired-looking rows. WFMU episodes have a null air window, so this is derived
// from air_date vs the station's local today (tz pinned to UTC, ±2-day margin).
func (suite *RadioServiceIntegrationTestSuite) TestGetEpisodes_FlagsUpcoming() {
	station := suite.createStation("KUPC")
	suite.Require().NoError(suite.db.Model(&catalogm.RadioStation{}).
		Where("id = ?", station.ID).Update("timezone", "UTC").Error)
	show := suite.createShow(station.ID, "Upcoming Show")
	now := time.Now().UTC()
	past := now.AddDate(0, 0, -2).Format("2006-01-02")
	today := now.Format("2006-01-02")
	future := now.AddDate(0, 0, 2).Format("2006-01-02")
	suite.createEpisode(show.ID, past)
	suite.createEpisode(show.ID, today)
	suite.createEpisode(show.ID, future)

	episodes, total, err := suite.radioService.GetEpisodes(show.ID, 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(3), total, "archive still lists upcoming episodes (labeled, not hidden)")
	suite.Require().Len(episodes, 3)

	upcomingByDate := map[string]bool{}
	for _, e := range episodes {
		upcomingByDate[e.AirDate] = e.IsUpcoming
	}
	suite.True(upcomingByDate[future], "a future-dated episode is flagged upcoming")
	suite.False(upcomingByDate[today], "today's episode is not upcoming")
	suite.False(upcomingByDate[past], "a past episode is not upcoming")
}

func (suite *RadioServiceIntegrationTestSuite) TestGetStationEpisodes_StrictPerStation() {
	flagship, sibling, standalone := suite.createNetworkFamily()

	flagShow := suite.createShow(flagship.ID, "Flag Show")
	sibShow := suite.createShow(sibling.ID, "Sib Show")
	aloneShow := suite.createShow(standalone.ID, "Alone Show")
	suite.createAiredEpisode(flagShow.ID, "2026-06-08")
	suite.createAiredEpisode(sibShow.ID, "2026-06-09")
	suite.createAiredEpisode(aloneShow.ID, "2026-06-07")

	// A flagship's feed contains ONLY its own episodes — never its network
	// siblings' (PSY-1074 reversed the PSY-1048 flagship-aggregates rule).
	rows, total, err := suite.radioService.GetStationEpisodes(flagship.ID, 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(rows, 1)
	suite.Equal("Flag Show", rows[0].ShowName)
	suite.Equal("Flagship", rows[0].StationName)
	suite.Equal("flagship", rows[0].StationSlug)
	suite.Equal("2026-06-08", rows[0].AirDate)
	suite.Require().NotNil(rows[0].ArtistPreview, "episodes without plays must serialize artist_preview as [], not null")
	suite.Empty(rows[0].ArtistPreview)

	// A channel's feed contains only its own episodes.
	sibRows, sibTotal, err := suite.radioService.GetStationEpisodes(sibling.ID, 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), sibTotal)
	suite.Require().Len(sibRows, 1)
	suite.Equal("Sib Show", sibRows[0].ShowName)
	suite.Equal("channel-two", sibRows[0].StationSlug)

	// Unknown station -> not found error.
	_, _, err = suite.radioService.GetStationEpisodes(99999, 10, 0)
	suite.Require().Error(err)
}

// TestGetStationEpisodes_WFMUFamilyStrictPerStation is the WFMU-family
// counterpart to TestGetStationEpisodes_StrictPerStation (PSY-1127). It seeds
// the real seeded WFMU slugs (flagship wfmu 91.1 + the three stream-only
// sub-channels) rather than the synthetic createNetworkFamily() fixture, and
// asserts the Latest-Playlists feed is strictly scoped to each station's own
// station_id.
//
// The leak this guards against: before PSY-1073 the WFMU import assigned the
// whole DJ index to every family station, so sub-stream shows existed as
// duplicate rows under the WFMU 91.1 flagship and leaked into its "Latest
// playlists" tab. PSY-1074 made GetStationEpisodes strictly per-station; this
// pins that behaviour to the actual WFMU station family so a future change
// can't quietly reintroduce flagship-aggregation for the real stations.
func (suite *RadioServiceIntegrationTestSuite) TestGetStationEpisodes_WFMUFamilyStrictPerStation() {
	_, stations := suite.seedWFMUNetwork()
	flagship := stations["wfmu"]
	drummer := stations["wfmu-drummer"]
	rockSoul := stations["wfmu-rocknsoulradio"]
	sheena := stations["wfmu-sheena"]

	// One show + episode per station, each on its own station_id.
	flagShow := suite.createShow(flagship.ID, "Three Chord Monte")
	drummerShow := suite.createShow(drummer.ID, "Give the Drummer Radio")
	rockSoulShow := suite.createShow(rockSoul.ID, "Rock'n'Soul Radio")
	sheenaShow := suite.createShow(sheena.ID, "Sheena's Jungle Room")
	suite.createAiredEpisode(flagShow.ID, "2026-06-08")
	suite.createAiredEpisode(drummerShow.ID, "2026-06-09")
	suite.createAiredEpisode(rockSoulShow.ID, "2026-06-10")
	suite.createAiredEpisode(sheenaShow.ID, "2026-06-11")

	// The flagship's feed contains ONLY its own episode — never the three
	// sub-stream channels' (the PSY-1127 leak).
	rows, total, err := suite.radioService.GetStationEpisodes(flagship.ID, 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total, "WFMU 91.1 must surface exactly its own playlists, no sub-stream leakage")
	suite.Require().Len(rows, 1)
	suite.Equal("Three Chord Monte", rows[0].ShowName)
	suite.Equal("wfmu", rows[0].StationSlug)

	// Each sub-stream's feed contains only its own episode.
	for _, tc := range []struct {
		station  catalogm.RadioStation
		showName string
		slug     string
	}{
		{drummer, "Give the Drummer Radio", "wfmu-drummer"},
		{rockSoul, "Rock'n'Soul Radio", "wfmu-rocknsoulradio"},
		{sheena, "Sheena's Jungle Room", "wfmu-sheena"},
	} {
		subRows, subTotal, err := suite.radioService.GetStationEpisodes(tc.station.ID, 50, 0)
		suite.Require().NoError(err)
		suite.Equal(int64(1), subTotal, "sub-stream %s must surface only its own playlists", tc.slug)
		suite.Require().Len(subRows, 1)
		suite.Equal(tc.showName, subRows[0].ShowName)
		suite.Equal(tc.slug, subRows[0].StationSlug)
	}
}

// TestGetStationEpisodes_SameDayOrderedByAirWindow pins the PSY-1297 feed
// ordering: within one air_date, episodes sort by the frozen air window
// (starts_at DESC — latest-aired first, catch-up semantics), NOT by insertion
// order. Windowless rows (pop-ups/off-schedule airings the window stamper
// deliberately skips) sort after the windowed ones via NULLS LAST, and a newer
// air_date still dominates everything.
func (suite *RadioServiceIntegrationTestSuite) TestGetStationEpisodes_SameDayOrderedByAirWindow() {
	station := suite.createStation("Order FM")
	// One show per slot — same-day feed rows are distinct shows in reality, and
	// the (show_id, air_date, external_id) unique index forbids same-show dupes.
	morningShow := suite.createShow(station.ID, "Morning Show")
	middayShow := suite.createShow(station.ID, "Midday Show")
	eveningShow := suite.createShow(station.ID, "Evening Show")
	popupShow := suite.createShow(station.ID, "Pop-up Show")
	nextDayShow := suite.createShow(station.ID, "Next Day Show")

	day := time.Now().UTC().AddDate(0, 0, -2)
	airDate := day.Format("2006-01-02")
	window := func(hour int) (*time.Time, *time.Time) {
		starts := time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, time.UTC)
		ends := starts.Add(time.Hour)
		return &starts, &ends
	}

	// Insertion order deliberately contradicts air order: morning first,
	// evening second, midday third — id DESC alone would read
	// midday, evening, morning.
	mStarts, mEnds := window(6)
	morning := suite.createEpisodeWindowed(morningShow.ID, airDate, mStarts, mEnds, 10)
	eStarts, eEnds := window(21)
	evening := suite.createEpisodeWindowed(eveningShow.ID, airDate, eStarts, eEnds, 10)
	dStarts, dEnds := window(12)
	midday := suite.createEpisodeWindowed(middayShow.ID, airDate, dStarts, dEnds, 10)
	// Windowless pop-up on the same day; needs plays to be feed-visible (PSY-1285).
	popup := suite.createEpisodeWindowed(popupShow.ID, airDate, nil, nil, 3)
	// A newer calendar day beats every same-day window. createAiredEpisode
	// stamps starts_at = now-72h — deliberately OLDER than every same-day
	// window above — which is what proves air_date dominates starts_at.
	newer := suite.createAiredEpisode(nextDayShow.ID, day.AddDate(0, 0, 1).Format("2006-01-02"))

	rows, total, err := suite.radioService.GetStationEpisodes(station.ID, 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Require().Len(rows, 5)
	gotIDs := []uint{rows[0].ID, rows[1].ID, rows[2].ID, rows[3].ID, rows[4].ID}
	suite.Equal([]uint{newer.ID, evening.ID, midday.ID, morning.ID, popup.ID}, gotIDs,
		"feed must order air_date DESC, then starts_at DESC NULLS LAST, not import order")

	// PSY-1298: feed rows expose the frozen air window so the frontend can
	// render viewer-local time blocks; windowless rows carry nil.
	suite.Require().NotNil(rows[1].StartsAt)
	suite.Require().NotNil(rows[1].EndsAt)
	suite.True(rows[1].StartsAt.Equal(*eStarts), "starts_at must round-trip through the feed")
	suite.True(rows[1].EndsAt.Equal(*eEnds), "ends_at must round-trip through the feed")
	suite.Nil(rows[4].StartsAt, "windowless pop-up must expose a nil window")
	suite.Nil(rows[4].EndsAt)
}

// TestGetEpisodes_SameDayAiredBeatsFutureWindow pins the future-window sink on
// the UNGATED per-show archive (PSY-1297): GetEpisodes shows upcoming rows by
// design (PSY-1205), so without the sink a pre-published later-today episode
// (future starts_at = the largest timestamp) would deterministically become
// episodes[0] — which drives the show page's "latest" pick, whose is_upcoming
// check is day-granular and can't skip a today-dated future row.
func (suite *RadioServiceIntegrationTestSuite) TestGetEpisodes_SameDayAiredBeatsFutureWindow() {
	station := suite.createStation("Archive FM")
	show := suite.createShow(station.ID, "Twice A Day")

	now := time.Now().UTC()
	airDate := now.Format("2006-01-02")
	extID := func(s string) *string { return &s }

	// Pre-published placeholder for a slot later today, imported FIRST (lower id)...
	futureStarts := now.Add(2 * time.Hour)
	futureEnds := now.Add(3 * time.Hour)
	future := &catalogm.RadioEpisode{
		ShowID: show.ID, AirDate: airDate,
		StartsAt: &futureStarts, EndsAt: &futureEnds,
		ExternalID: extID("tonight"),
	}
	suite.Require().NoError(suite.db.Create(future).Error)
	// ...then the already-aired morning episode, imported after airing (higher id).
	airedStarts := now.Add(-4 * time.Hour)
	airedEnds := now.Add(-3 * time.Hour)
	aired := &catalogm.RadioEpisode{
		ShowID: show.ID, AirDate: airDate,
		StartsAt: &airedStarts, EndsAt: &airedEnds,
		PlayCount: 12, ExternalID: extID("morning"),
	}
	suite.Require().NoError(suite.db.Create(aired).Error)

	episodes, total, err := suite.radioService.GetEpisodes(show.ID, 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Require().Len(episodes, 2)
	suite.Equal(aired.ID, episodes[0].ID,
		"same-day aired episode must outrank the not-yet-aired future window for episodes[0]")
	suite.Equal(future.ID, episodes[1].ID)
}

// TestGetEpisodeByShowAndDate_SameDaySiblingsResolveToListWinner pins the
// PSY-1297 by-date resolution: with same-day siblings, the day-keyed lookup
// returns the row the list surfaces rank first. The three-sibling insertion
// order is chosen so every plausible mutant fails: future is the LOWEST id
// (an unordered First — primary-key ASC, i.e. a revert of the Order call —
// picks it), popup is the HIGHEST id (the old air_date DESC, id DESC clause
// picks it), and a sink-less starts_at DESC also picks future. Only the full
// shipped clause resolves to aired. Also exercises GORM's First composing
// with the custom raw Order (First appends a primary-key ASC key AFTER ours;
// unreachable behind id DESC).
func (suite *RadioServiceIntegrationTestSuite) TestGetEpisodeByShowAndDate_SameDaySiblingsResolveToListWinner() {
	station := suite.createStation("Lookup FM")
	show := suite.createShow(station.ID, "Doubleheader")

	now := time.Now().UTC()
	airDate := now.Format("2006-01-02")
	extID := func(s string) *string { return &s }

	// 1st insert (lowest id): pre-published later-today sibling.
	futureStarts := now.Add(2 * time.Hour)
	futureEnds := now.Add(3 * time.Hour)
	future := &catalogm.RadioEpisode{
		ShowID: show.ID, AirDate: airDate,
		StartsAt: &futureStarts, EndsAt: &futureEnds,
		ExternalID: extID("tonight"),
	}
	suite.Require().NoError(suite.db.Create(future).Error)
	// 2nd insert: the aired windowed episode — the expected winner.
	airedStarts := now.Add(-4 * time.Hour)
	airedEnds := now.Add(-3 * time.Hour)
	aired := &catalogm.RadioEpisode{
		ShowID: show.ID, AirDate: airDate,
		StartsAt: &airedStarts, EndsAt: &airedEnds,
		PlayCount: 7, ExternalID: extID("aired"),
	}
	suite.Require().NoError(suite.db.Create(aired).Error)
	// 3rd insert (highest id): windowless pop-up with plays.
	popup := &catalogm.RadioEpisode{
		ShowID: show.ID, AirDate: airDate,
		PlayCount: 4, ExternalID: extID("popup"),
	}
	suite.Require().NoError(suite.db.Create(popup).Error)

	got, err := suite.radioService.GetEpisodeByShowAndDate(show.ID, airDate)
	suite.Require().NoError(err)
	suite.Equal(aired.ID, got.ID,
		"day-keyed lookup must resolve to the aired windowed sibling (the list surfaces' same-day winner)")
}

// TestLatestEpisodeForShow_SameDayWindowedBeatsWindowlessPopup pins the
// accepted PSY-1297 tradeoff on the LIMIT-1 now-playing selector: when a show
// has, on one day, both a windowed slot episode and a windowless off-schedule
// extra (imported later, higher id), NULLS LAST picks the windowed one — the
// scheduled playlist is the archive fallback's "latest". The old id-DESC
// ordering picked whichever imported last.
func (suite *RadioServiceIntegrationTestSuite) TestLatestEpisodeForShow_SameDayWindowedBeatsWindowlessPopup() {
	station := suite.createStation("Fallback FM")
	show := suite.createShow(station.ID, "Slot And Popup")

	day := time.Now().UTC().AddDate(0, 0, -1)
	airDate := day.Format("2006-01-02")
	extID := func(s string) *string { return &s }

	starts := time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, time.UTC)
	ends := starts.Add(time.Hour)
	windowed := &catalogm.RadioEpisode{
		ShowID: show.ID, AirDate: airDate,
		StartsAt: &starts, EndsAt: &ends,
		PlayCount: 8, ExternalID: extID("slot"),
	}
	suite.Require().NoError(suite.db.Create(windowed).Error)
	popup := &catalogm.RadioEpisode{
		ShowID: show.ID, AirDate: airDate,
		PlayCount: 5, ExternalID: extID("popup"),
	}
	suite.Require().NoError(suite.db.Create(popup).Error)

	got, err := suite.radioService.latestEpisodeForShow(show.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(got)
	suite.Equal(windowed.ID, got.ID,
		"windowed slot episode wins over the same-day windowless pop-up (accepted PSY-1297 tradeoff)")
}

func (suite *RadioServiceIntegrationTestSuite) TestGetRecentEpisodes_ActiveStationsOnly() {
	active := suite.createStation("Active FM")
	dormant := suite.createStation("Dormant FM")
	suite.Require().NoError(suite.db.Model(&catalogm.RadioStation{}).Where("id = ?", dormant.ID).Update("is_active", false).Error)

	activeShow := suite.createShow(active.ID, "Active Show")
	dormantShow := suite.createShow(dormant.ID, "Dormant Show")
	suite.createAiredEpisode(activeShow.ID, "2026-06-08")
	suite.createAiredEpisode(activeShow.ID, "2026-06-09")
	suite.createAiredEpisode(dormantShow.ID, "2026-06-09")

	rows, total, err := suite.radioService.GetRecentEpisodes(10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Require().Len(rows, 2)
	suite.Equal("2026-06-09", rows[0].AirDate)
	suite.Equal("2026-06-08", rows[1].AirDate)
	for _, r := range rows {
		suite.Equal("Active Show", r.ShowName)
	}

	// Pagination: second page carries the remaining row.
	page2, total2, err := suite.radioService.GetRecentEpisodes(1, 1)
	suite.Require().NoError(err)
	suite.Equal(int64(2), total2)
	suite.Require().Len(page2, 1)
	suite.Equal("2026-06-08", page2[0].AirDate)
}

// TestGetStationEpisodes_AirWindowGate pins the air-window aired-only contract
// (PSY-1285, superseding PSY-1204's day-granular bound): the "Latest playlists"
// feed shows an episode iff its frozen window has passed (starts_at <= now) OR it
// already has plays — so a not-yet-aired (future-windowed) broadcast AND a
// windowless 0-track placeholder are both hidden, while a windowed-aired episode
// and any episode carrying a playlist surface. Determinism: explicit window
// instants (not date strings) drive the gate, so no host-vs-DB midnight skew; the
// date strings only feed the coarse pre-filter and stay clear of the boundary.
func (suite *RadioServiceIntegrationTestSuite) TestGetStationEpisodes_AirWindowGate() {
	station := suite.createStation("Aired FM")
	suite.Require().NoError(suite.db.Model(&catalogm.RadioStation{}).
		Where("id = ?", station.ID).Update("timezone", "UTC").Error)
	show := suite.createShow(station.ID, "Catch-Up Show")

	now := time.Now().UTC()
	ptr := func(t time.Time) *time.Time { return &t }
	airedWindowed := now.AddDate(0, 0, -2).Format("2006-01-02")
	withPlays := now.AddDate(0, 0, -4).Format("2006-01-02")
	emptyWindowless := now.AddDate(0, 0, -3).Format("2006-01-02")
	notYetAired := now.Format("2006-01-02")
	futureDated := now.AddDate(0, 0, 2).Format("2006-01-02")

	// SHOWN: window already passed, OR (windowless but) has a playlist.
	suite.createEpisodeWindowed(show.ID, airedWindowed, ptr(now.Add(-50*time.Hour)), ptr(now.Add(-47*time.Hour)), 0)
	suite.createEpisodeWindowed(show.ID, withPlays, nil, nil, 3)
	// HIDDEN: not-yet-aired is gated by its FUTURE window even with an early play snapshot
	// (a windowed episode is gated by its window, never by play_count); plus a windowless
	// 0-track placeholder and a future-dated placeholder.
	suite.createEpisodeWindowed(show.ID, notYetAired, ptr(now.Add(3*time.Hour)), ptr(now.Add(5*time.Hour)), 5)
	suite.createEpisode(show.ID, emptyWindowless)
	suite.createEpisode(show.ID, futureDated)

	rows, total, err := suite.radioService.GetStationEpisodes(station.ID, 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(2), total, "only aired-windowed + has-plays episodes count")
	suite.Require().Len(rows, 2)

	got := map[string]bool{}
	for _, r := range rows {
		got[r.AirDate] = true
	}
	suite.True(got[airedWindowed], "an episode whose window has passed is included")
	suite.True(got[withPlays], "a windowless episode with a playlist is included")
	suite.False(got[notYetAired], "a not-yet-aired (future-windowed) episode is hidden even with an early play snapshot")
	suite.False(got[emptyWindowless], "a windowless 0-track placeholder is hidden")
	suite.False(got[futureDated], "a future-dated placeholder is hidden")
}

// TestGetRecentEpisodes_AirWindowGate is the dial-wide-feed counterpart:
// GetRecentEpisodes shares episodeRows with GetStationEpisodes, so the air-window
// aired-only gate must apply there too (PSY-1285). This station is left with the
// default (NULL) timezone, so it also covers the COALESCE-to-UTC fallback path that
// the common createStation case relies on.
func (suite *RadioServiceIntegrationTestSuite) TestGetRecentEpisodes_AirWindowGate() {
	station := suite.createStation("Dial FM") // default timezone: NULL → UTC fallback
	show := suite.createShow(station.ID, "Dial Show")

	now := time.Now().UTC()
	ptr := func(t time.Time) *time.Time { return &t }
	aired := now.AddDate(0, 0, -2).Format("2006-01-02")
	future := now.AddDate(0, 0, 2).Format("2006-01-02")
	// Aired (window passed) → surfaces; future-dated windowless placeholder → hidden.
	suite.createEpisodeWindowed(show.ID, aired, ptr(now.Add(-50*time.Hour)), ptr(now.Add(-47*time.Hour)), 0)
	suite.createEpisode(show.ID, future)

	rows, total, err := suite.radioService.GetRecentEpisodes(50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total, "the not-yet-aired placeholder is excluded from the dial-wide total")
	suite.Require().Len(rows, 1)
	suite.Equal(aired, rows[0].AirDate, "only the aired episode surfaces; NULL tz falls back to UTC")
}

// TestGetStationEpisodes_ToleratesBadStationTimezone guards the crash-proof SQL
// (PSY-1204, adversarial-review CRITICAL): a legacy/garbage timezone stored
// directly (bypassing normalizeStationTimezone) must NOT error out of AT TIME
// ZONE and 500 the public feed — pg_timezone_names resolves the unknown zone to
// UTC. Without the fallback this query raises `time zone "..." not recognized`.
func (suite *RadioServiceIntegrationTestSuite) TestGetStationEpisodes_ToleratesBadStationTimezone() {
	station := suite.createStation("Garbage TZ FM")
	// Write a non-loadable zone straight to the column (the service would reject
	// it; this simulates a legacy/out-of-band row).
	suite.Require().NoError(suite.db.Model(&catalogm.RadioStation{}).
		Where("id = ?", station.ID).Update("timezone", "Mars/Olympus").Error)
	show := suite.createShow(station.ID, "Resilient Show")

	now := time.Now().UTC()
	ptr := func(t time.Time) *time.Time { return &t }
	past := now.AddDate(0, 0, -2).Format("2006-01-02")
	future := now.AddDate(0, 0, 2).Format("2006-01-02")
	// Aired-windowed so it passes the PSY-1285 air-window gate (the point here is the
	// bad-timezone SQL doesn't error, not the gate itself); future stays hidden.
	suite.createEpisodeWindowed(show.ID, past, ptr(now.Add(-50*time.Hour)), ptr(now.Add(-47*time.Hour)), 0)
	suite.createEpisode(show.ID, future)

	rows, total, err := suite.radioService.GetStationEpisodes(station.ID, 50, 0)
	suite.Require().NoError(err, "a bad stored timezone must not error the feed")
	suite.Equal(int64(1), total)
	suite.Require().Len(rows, 1)
	suite.Equal(past, rows[0].AirDate, "unknown zone falls back to UTC; future still excluded")
}

// TestNormalizeStationTimezone covers the write-boundary validator (PSY-1204):
// it validates against the SAME catalog the feed resolves through
// (pg_timezone_names), so an accepted value can never silently degrade to UTC in
// the feed. nil/blank → nil (NULL → UTC), canonical IANA is accepted and its
// casing normalized, and values Go's time.LoadLocation would accept but Postgres'
// catalog lacks (abbreviations like "EST", the alias "Local") are rejected.
func (suite *RadioServiceIntegrationTestSuite) TestNormalizeStationTimezone() {
	tz := func(s string) *string { return &s }

	for _, in := range []*string{nil, tz(""), tz("   ")} {
		got, err := suite.radioService.normalizeStationTimezone(in)
		suite.Require().NoError(err)
		suite.Nil(got, "nil/blank normalizes to NULL")
	}

	for _, tc := range []struct{ in, want string }{
		{"America/New_York", "America/New_York"},
		{"  america/new_york ", "America/New_York"}, // trimmed + canonical casing
		{"UTC", "UTC"},
	} {
		got, err := suite.radioService.normalizeStationTimezone(tz(tc.in))
		suite.Require().NoError(err)
		suite.Require().NotNil(got)
		suite.Equal(tc.want, *got)
	}

	for _, bad := range []string{"EST", "Local", "Mars/Olympus", "not a zone"} {
		got, err := suite.radioService.normalizeStationTimezone(tz(bad))
		suite.Require().Error(err, "should reject %q (not a pg_timezone_names entry)", bad)
		suite.Nil(got)
	}
}

func (suite *RadioServiceIntegrationTestSuite) TestGetTopArtistsForStation_StrictPerStation() {
	flagship, sibling, standalone := suite.createNetworkFamily()
	flagShow := suite.createShow(flagship.ID, "Flag Show")
	sibShow := suite.createShow(sibling.ID, "Sib Show")
	aloneShow := suite.createShow(standalone.ID, "Alone Show")
	flagEp := suite.createEpisode(flagShow.ID, "2026-06-08")
	sibEp := suite.createEpisode(sibShow.ID, "2026-06-09")
	aloneEp := suite.createEpisode(aloneShow.ID, "2026-06-07")

	suite.createPlay(flagEp.ID, 1, "Shared Artist")
	suite.createPlay(sibEp.ID, 1, "Shared Artist")
	suite.createPlay(sibEp.ID, 2, "Channel Artist")
	suite.createPlay(aloneEp.ID, 1, "Outsider Artist")

	// A flagship's top artists count ONLY its own plays — sibling channels'
	// plays are excluded (PSY-1074 reversed the PSY-1048 network aggregation).
	artists, err := suite.radioService.GetTopArtistsForStation(flagship.ID, 90, 10)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 1)
	suite.Equal("Shared Artist", artists[0].ArtistName)
	suite.Equal(1, artists[0].PlayCount)
	suite.Equal(1, artists[0].EpisodeCount)

	// A channel's top artists count only its own plays. Both have 1 play, so
	// assert membership rather than tie-break order.
	sibArtists, err := suite.radioService.GetTopArtistsForStation(sibling.ID, 90, 10)
	suite.Require().NoError(err)
	suite.Require().Len(sibArtists, 2)
	sibNames := []string{sibArtists[0].ArtistName, sibArtists[1].ArtistName}
	suite.ElementsMatch([]string{"Shared Artist", "Channel Artist"}, sibNames)

	// Unknown station -> not found error.
	_, err = suite.radioService.GetTopArtistsForStation(99999, 90, 10)
	suite.Require().Error(err)
}

func (suite *RadioServiceIntegrationTestSuite) TestGetTopLabelsForStation_StrictPerStation() {
	flagship, sibling, _ := suite.createNetworkFamily()
	flagShow := suite.createShow(flagship.ID, "Flag Show")
	sibShow := suite.createShow(sibling.ID, "Sib Show")
	flagEp := suite.createEpisode(flagShow.ID, "2026-06-08")
	sibEp := suite.createEpisode(sibShow.ID, "2026-06-09")

	label := "Family Label"
	other := "Channel Label"
	plays := []catalogm.RadioPlay{
		{EpisodeID: flagEp.ID, Position: 1, ArtistName: "A", LabelName: &label},
		{EpisodeID: sibEp.ID, Position: 1, ArtistName: "B", LabelName: &label},
		{EpisodeID: sibEp.ID, Position: 2, ArtistName: "C", LabelName: &other},
	}
	for i := range plays {
		suite.Require().NoError(suite.db.Create(&plays[i]).Error)
	}

	// A flagship's top labels count ONLY its own plays (PSY-1074).
	labels, err := suite.radioService.GetTopLabelsForStation(flagship.ID, 90, 10)
	suite.Require().NoError(err)
	suite.Require().Len(labels, 1)
	suite.Equal("Family Label", labels[0].LabelName)
	suite.Equal(1, labels[0].PlayCount)

	// A channel's top labels count only its own plays.
	sibLabels, err := suite.radioService.GetTopLabelsForStation(sibling.ID, 90, 10)
	suite.Require().NoError(err)
	suite.Require().Len(sibLabels, 2)

	// Unknown station -> not found error.
	_, err = suite.radioService.GetTopLabelsForStation(99999, 90, 10)
	suite.Require().Error(err)
}

func TestRadioService_NilDB_ResolveStationID(t *testing.T) {
	svc := &RadioService{db: nil}
	assertNilDBError(t, func() error { _, err := svc.ResolveStationIDBySlug("x"); return err })
}

func (suite *RadioServiceIntegrationTestSuite) TestResolveStationIDBySlug() {
	station := suite.createStation("Resolve FM")

	id, err := suite.radioService.ResolveStationIDBySlug(station.Slug)
	suite.Require().NoError(err)
	suite.Equal(station.ID, id)

	_, err = suite.radioService.ResolveStationIDBySlug("no-such-station")
	suite.Require().Error(err)
}

func (suite *RadioServiceIntegrationTestSuite) TestUpdateStation_DuplicateNameRejected() {
	suite.createStation("KEXP")
	other := suite.createStation("WFMU")

	// Renaming WFMU to an existing name must return a clean conflict, not a 500
	// (PSY-1131 — symmetric with the create path).
	name := "KEXP"
	_, err := suite.radioService.UpdateStation(other.ID, &contracts.UpdateRadioStationRequest{
		Name: &name,
	})

	suite.Require().Error(err)
	var radioErr *apperrors.RadioError
	suite.ErrorAs(err, &radioErr)
	suite.Equal(apperrors.CodeRadioStationNameConflict, radioErr.Code)
}

func (suite *RadioServiceIntegrationTestSuite) TestCreateShow_InvalidScheduleRejected() {
	station := suite.createStation("KEXP")

	// A malformed schedule (empty timezone) is rejected at the write boundary
	// with a schedule-invalid error mapped to 422 (PSY-1131).
	bad := json.RawMessage(`{"timezone":"","slots":[]}`)
	_, err := suite.radioService.CreateShow(station.ID, &contracts.CreateRadioShowRequest{
		Name:     "Bad Schedule Show",
		Schedule: &bad,
	})

	suite.Require().Error(err)
	var radioErr *apperrors.RadioError
	suite.ErrorAs(err, &radioErr)
	suite.Equal(apperrors.CodeRadioScheduleInvalid, radioErr.Code)
}
