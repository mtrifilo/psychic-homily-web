package catalog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
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
	assertNilDBError(t, func() error { _, err := svc.ListShows(1); return err })
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

func (suite *RadioServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM radio_artist_affinity")
	_, _ = sqlDB.Exec("DELETE FROM radio_plays")
	_, _ = sqlDB.Exec("DELETE FROM radio_episodes")
	_, _ = sqlDB.Exec("DELETE FROM radio_shows")
	_, _ = sqlDB.Exec("DELETE FROM radio_stations")
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
		BroadcastType: models.BroadcastTypeBoth,
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

func (suite *RadioServiceIntegrationTestSuite) createEpisode(showID uint, airDate string) *models.RadioEpisode {
	ep := &models.RadioEpisode{
		ShowID:  showID,
		AirDate: airDate,
	}
	err := suite.db.Create(ep).Error
	suite.Require().NoError(err)
	return ep
}

func (suite *RadioServiceIntegrationTestSuite) createPlay(episodeID uint, position int, artistName string) *models.RadioPlay {
	play := &models.RadioPlay{
		EpisodeID:  episodeID,
		Position:   position,
		ArtistName: artistName,
	}
	err := suite.db.Create(play).Error
	suite.Require().NoError(err)
	return play
}

func (suite *RadioServiceIntegrationTestSuite) createArtist(name string) *models.Artist {
	slug := utils.GenerateArtistSlug(name)
	artist := &models.Artist{Name: name, Slug: &slug}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *RadioServiceIntegrationTestSuite) createRelease(title string) *models.Release {
	release := &models.Release{Title: title}
	err := suite.db.Create(release).Error
	suite.Require().NoError(err)
	return release
}

func (suite *RadioServiceIntegrationTestSuite) createLabel(name string) *models.Label {
	label := &models.Label{Name: name, Status: models.LabelStatusActive}
	err := suite.db.Create(label).Error
	suite.Require().NoError(err)
	return label
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
		BroadcastType: models.BroadcastTypeBoth,
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
		BroadcastType: models.BroadcastTypeInternet,
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

func (suite *RadioServiceIntegrationTestSuite) TestCreateStation_UniqueSlugCollision() {
	suite.createStation("KEXP")

	resp, err := suite.radioService.CreateStation(&contracts.CreateRadioStationRequest{
		Name:          "KEXP",
		BroadcastType: models.BroadcastTypeInternet,
	})

	suite.Require().NoError(err)
	suite.Contains(resp.Slug, "kexp-") // should get a suffix
	suite.NotEqual("kexp", resp.Slug)
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

	resp, err := suite.radioService.ListShows(station.ID)

	suite.Require().NoError(err)
	suite.Len(resp, 2)
	// Ordered by name ASC
	suite.Equal("Afternoon Show", resp[0].Name)
	suite.Equal(int64(0), resp[0].EpisodeCount)
	suite.Equal("Morning Show", resp[1].Name)
	suite.Equal(int64(2), resp[1].EpisodeCount)
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
	linkedPlay := &models.RadioPlay{
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
	play1 := &models.RadioPlay{EpisodeID: ep.ID, Position: 0, ArtistName: "A", LabelName: &labelName}
	play2 := &models.RadioPlay{EpisodeID: ep.ID, Position: 1, ArtistName: "B", LabelName: &labelName}
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
	play := &models.RadioPlay{
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
	play := &models.RadioPlay{
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
	play := &models.RadioPlay{
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
	play1 := &models.RadioPlay{EpisodeID: ep1.ID, Position: 0, ArtistName: "Radiohead", AlbumTitle: &album, IsNew: true}
	suite.Require().NoError(suite.db.Create(play1).Error)

	// Only one station, cross-station query should return nothing
	resp, err := suite.radioService.GetNewReleaseRadar(0, 10)
	suite.Require().NoError(err)
	suite.Empty(resp)

	// Add second station playing same new release
	station2 := suite.createStation("WFMU")
	show2 := suite.createShow(station2.ID, "WFMU Show")
	ep2 := suite.createEpisode(show2.ID, "2026-01-02")
	play2 := &models.RadioPlay{EpisodeID: ep2.ID, Position: 0, ArtistName: "Radiohead", AlbumTitle: &album, IsNew: true}
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
	linked := &models.RadioPlay{EpisodeID: ep.ID, Position: 0, ArtistName: "Radiohead", ArtistID: &artist.ID}
	unlinked := &models.RadioPlay{EpisodeID: ep.ID, Position: 1, ArtistName: "Unknown"}
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
	suite.db.Model(&models.RadioShow{}).Where("station_id = ?", station.ID).Count(&showCount)
	suite.Equal(int64(0), showCount)

	var epCount int64
	suite.db.Model(&models.RadioEpisode{}).Where("show_id = ?", show.ID).Count(&epCount)
	suite.Equal(int64(0), epCount)

	var playCount int64
	suite.db.Model(&models.RadioPlay{}).Where("episode_id = ?", ep.ID).Count(&playCount)
	suite.Equal(int64(0), playCount)
}
