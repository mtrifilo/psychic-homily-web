package catalog

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/utils"
)

// =============================================================================
// UNIT TESTS: KEXP response parsing
// =============================================================================

func TestParseReleaseYear(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"2026", 2026},
		{"2026-01-15", 2026},
		{"2026-01-15T00:00:00Z", 2026},
		{"1998", 1998},
		{"", 0},
		{"abc", 0},
		{"12", 0},        // too short
		{"0000", 0},      // out of range
		{"9999", 0},      // out of range
		{"1899", 0},      // out of range (< 1900)
		{"2101", 0},      // out of range (> 2100)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseReleaseYear(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseKEXPPlay_FullFields(t *testing.T) {
	kPlay := kexpPlay{
		ID:                     12345,
		PlayType:               "trackplay",
		Airdate:                "2026-01-15T10:30:00Z",
		Artist:                 "Radiohead",
		Song:                   "Everything In Its Right Place",
		Album:                  "Kid A",
		Label:                  "Parlophone",
		ReleaseDate:            "2000-10-02",
		RotationStatus:         "heavy",
		IsNew:                  true,
		IsLive:                 false,
		IsRequest:              true,
		Comment:                "Classic album opener",
		MusicBrainzArtistID:    "a74b1b7f-71a5-4011-9441-d0b5e4122711",
		MusicBrainzReleaseID:   "b95ce3ff-3d05-4e87-9e01-c97b66af13d4",
		MusicBrainzRecordingID: "c22af0e6-1e3c-4b6c-aa83-42a2de43b84c",
	}

	play := parseKEXPPlay(kPlay, 5)

	assert.Equal(t, 5, play.Position)
	assert.Equal(t, "Radiohead", play.ArtistName)
	assert.Equal(t, "Everything In Its Right Place", *play.TrackTitle)
	assert.Equal(t, "Kid A", *play.AlbumTitle)
	assert.Equal(t, "Parlophone", *play.LabelName)
	assert.Equal(t, 2000, *play.ReleaseYear)
	assert.Equal(t, "heavy", *play.RotationStatus)
	assert.True(t, play.IsNew)
	assert.False(t, play.IsLivePerformance)
	assert.True(t, play.IsRequest)
	assert.Equal(t, "Classic album opener", *play.DJComment)
	assert.Equal(t, "a74b1b7f-71a5-4011-9441-d0b5e4122711", *play.MusicBrainzArtistID)
	assert.Equal(t, "b95ce3ff-3d05-4e87-9e01-c97b66af13d4", *play.MusicBrainzReleaseID)
	assert.Equal(t, "c22af0e6-1e3c-4b6c-aa83-42a2de43b84c", *play.MusicBrainzRecordingID)
	assert.NotNil(t, play.AirTimestamp)
	assert.Equal(t, "2026-01-15T10:30:00Z", play.AirTimestamp.Format(time.RFC3339))
}

func TestParseKEXPPlay_MinimalFields(t *testing.T) {
	kPlay := kexpPlay{
		PlayType: "trackplay",
		Artist:   "Sonic Youth",
	}

	play := parseKEXPPlay(kPlay, 0)

	assert.Equal(t, 0, play.Position)
	assert.Equal(t, "Sonic Youth", play.ArtistName)
	assert.Nil(t, play.TrackTitle)
	assert.Nil(t, play.AlbumTitle)
	assert.Nil(t, play.LabelName)
	assert.Nil(t, play.ReleaseYear)
	assert.Nil(t, play.RotationStatus)
	assert.Nil(t, play.DJComment)
	assert.False(t, play.IsNew)
	assert.False(t, play.IsLivePerformance)
	assert.False(t, play.IsRequest)
	assert.Nil(t, play.MusicBrainzArtistID)
	assert.Nil(t, play.AirTimestamp)
}

func TestParseKEXPEpisode(t *testing.T) {
	show := kexpShow{
		ID:          5678,
		ProgramID:   42,
		ProgramName: "The Morning Show",
		StartTime:   "2026-01-15T06:00:00-08:00",
		EndTime:     "2026-01-15T10:00:00-08:00",
		ArchiveURL:  "https://kexp.org/archive/2026-01-15",
	}

	ep := parseKEXPEpisode(show, "42")

	assert.Equal(t, "5678", ep.ExternalID)
	assert.Equal(t, "42", ep.ShowExternalID)
	assert.Equal(t, "2026-01-15", ep.AirDate)
	assert.NotNil(t, ep.AirTime)
	assert.Equal(t, "06:00:00", *ep.AirTime)
	assert.NotNil(t, ep.DurationMinutes)
	assert.Equal(t, 240, *ep.DurationMinutes)
	assert.NotNil(t, ep.Title)
	assert.Equal(t, "The Morning Show", *ep.Title)
	assert.NotNil(t, ep.ArchiveURL)
	assert.Equal(t, "https://kexp.org/archive/2026-01-15", *ep.ArchiveURL)
}

func TestParseKEXPEpisode_NoEndTime(t *testing.T) {
	show := kexpShow{
		ID:        9999,
		StartTime: "2026-01-15T06:00:00-08:00",
	}

	ep := parseKEXPEpisode(show, "1")

	assert.Equal(t, "9999", ep.ExternalID)
	assert.Equal(t, "2026-01-15", ep.AirDate)
	assert.Nil(t, ep.DurationMinutes)
}

// =============================================================================
// UNIT TESTS: KEXP provider with mock HTTP server
// =============================================================================

func TestKEXPProvider_DiscoverShows(t *testing.T) {
	mux := http.NewServeMux()

	// Mock hosts endpoint
	mux.HandleFunc("/v2/hosts/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"next":  nil,
			"count": 2,
			"results": []map[string]interface{}{
				{"id": 1, "name": "John Richards"},
				{"id": 2, "name": "Cheryl Waters"},
			},
		})
	})

	// Mock programs endpoint
	mux.HandleFunc("/v2/programs/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"next":  nil,
			"count": 2,
			"results": []map[string]interface{}{
				{
					"id":          42,
					"name":        "The Morning Show",
					"description": "Wake up with KEXP",
					"image_uri":   "https://kexp.org/morning.jpg",
					"host_ids":    []int{1},
					"is_active":   true,
				},
				{
					"id":        43,
					"name":      "The Midday Show",
					"host_ids":  []int{2},
					"is_active": true,
				},
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	shows, err := provider.DiscoverShows()

	require.NoError(t, err)
	assert.Len(t, shows, 2)

	assert.Equal(t, "42", shows[0].ExternalID)
	assert.Equal(t, "The Morning Show", shows[0].Name)
	assert.Equal(t, "Wake up with KEXP", *shows[0].Description)
	assert.Equal(t, "https://kexp.org/morning.jpg", *shows[0].ImageURL)
	assert.Equal(t, "John Richards", *shows[0].HostName)

	assert.Equal(t, "43", shows[1].ExternalID)
	assert.Equal(t, "The Midday Show", shows[1].Name)
	assert.Equal(t, "Cheryl Waters", *shows[1].HostName)
}

func TestKEXPProvider_DiscoverShows_Pagination(t *testing.T) {
	callCount := 0
	mux := http.NewServeMux()

	mux.HandleFunc("/v2/hosts/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"next": nil, "count": 0, "results": []interface{}{},
		})
	})

	var server *httptest.Server
	mux.HandleFunc("/v2/programs/", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"next":  fmt.Sprintf("%s/v2/programs/?offset=1", server.URL),
				"count": 2,
				"results": []map[string]interface{}{
					{"id": 1, "name": "Show One", "host_ids": []int{}},
				},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"next":  nil,
				"count": 2,
				"results": []map[string]interface{}{
					{"id": 2, "name": "Show Two", "host_ids": []int{}},
				},
			})
		}
	})

	server = httptest.NewServer(mux)
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	shows, err := provider.DiscoverShows()

	require.NoError(t, err)
	assert.Len(t, shows, 2)
	assert.Equal(t, "Show One", shows[0].Name)
	assert.Equal(t, "Show Two", shows[1].Name)
	assert.Equal(t, 2, callCount) // Two program pages fetched
}

func TestKEXPProvider_FetchNewEpisodes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/shows/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"next":  nil,
			"count": 1,
			"results": []map[string]interface{}{
				{
					"id":           5678,
					"program_id":   42,
					"program_name": "The Morning Show",
					"start_time":   "2026-01-15T06:00:00-08:00",
					"end_time":     "2026-01-15T10:00:00-08:00",
					"archive_url":  "https://kexp.org/archive/5678",
				},
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	episodes, err := provider.FetchNewEpisodes("42", since)

	require.NoError(t, err)
	assert.Len(t, episodes, 1)
	assert.Equal(t, "5678", episodes[0].ExternalID)
	assert.Equal(t, "42", episodes[0].ShowExternalID)
	assert.Equal(t, "2026-01-15", episodes[0].AirDate)
	assert.NotNil(t, episodes[0].DurationMinutes)
	assert.Equal(t, 240, *episodes[0].DurationMinutes)
}

func TestKEXPProvider_FetchPlaylist(t *testing.T) {
	var playsRequestQuery string

	mux := http.NewServeMux()
	// Show detail endpoint — called first to resolve start_time.
	mux.HandleFunc("/v2/shows/5678/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           5678,
			"program_id":   42,
			"program_name": "The Morning Show",
			"start_time":   "2026-01-15T06:00:00-08:00",
		})
	})
	mux.HandleFunc("/v2/plays/", func(w http.ResponseWriter, r *http.Request) {
		playsRequestQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(map[string]interface{}{
			"next":  nil,
			"count": 3,
			"results": []map[string]interface{}{
				{
					"id":                    1,
					"play_type":             "trackplay",
					"airdate":               "2026-01-15T06:05:00-08:00",
					"artist":                "Radiohead",
					"song":                  "Everything In Its Right Place",
					"album":                 "Kid A",
					"label_name":            "Parlophone",
					"release_date":          "2000-10-02",
					"rotation_status":       "library",
					"is_new":                false,
					"is_live":               false,
					"is_request":            false,
					"musicbrainz_artist_id": "a74b1b7f-71a5-4011-9441-d0b5e4122711",
				},
				{
					"id":        2,
					"play_type": "airbreak", // Should be skipped
					"airdate":   "2026-01-15T06:10:00-08:00",
				},
				{
					"id":         3,
					"play_type":  "trackplay",
					"airdate":    "2026-01-15T06:15:00-08:00",
					"artist":     "Deerhunter",
					"song":       "Desire Lines",
					"is_new":     true,
					"is_live":    true,
					"is_request": false,
				},
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	plays, err := provider.FetchPlaylist("5678")

	require.NoError(t, err)
	assert.Len(t, plays, 2) // Airbreak skipped

	// Verify the plays request used time-range filtering instead of show_id.
	assert.NotContains(t, playsRequestQuery, "show_id",
		"plays request must not include show_id (silently ignored by KEXP)")
	assert.NotContains(t, playsRequestQuery, "show=",
		"plays request must not include show= (silently ignored by KEXP)")
	assert.Contains(t, playsRequestQuery, "airdate_after=",
		"plays request must filter by airdate_after")
	assert.Contains(t, playsRequestQuery, "airdate_before=",
		"plays request must filter by airdate_before")
	assert.Contains(t, playsRequestQuery, "play_type=trackplay",
		"plays request must filter to trackplay entries only")
	// airdate_after should match the show start_time (UTC). The broadcast
	// starts at 2026-01-15T06:00:00-08:00 which is 2026-01-15T14:00:00Z.
	assert.Contains(t, playsRequestQuery, "airdate_after=2026-01-15T14:00:00Z")
	// airdate_before should be 5 hours later: 2026-01-15T19:00:00Z.
	assert.Contains(t, playsRequestQuery, "airdate_before=2026-01-15T19:00:00Z")

	// First play
	assert.Equal(t, 0, plays[0].Position)
	assert.Equal(t, "Radiohead", plays[0].ArtistName)
	assert.Equal(t, "Everything In Its Right Place", *plays[0].TrackTitle)
	assert.Equal(t, "Kid A", *plays[0].AlbumTitle)
	assert.Equal(t, "Parlophone", *plays[0].LabelName)
	assert.Equal(t, 2000, *plays[0].ReleaseYear)
	assert.Equal(t, "library", *plays[0].RotationStatus)
	assert.False(t, plays[0].IsNew)
	assert.Equal(t, "a74b1b7f-71a5-4011-9441-d0b5e4122711", *plays[0].MusicBrainzArtistID)

	// Second play
	assert.Equal(t, 1, plays[1].Position)
	assert.Equal(t, "Deerhunter", plays[1].ArtistName)
	assert.True(t, plays[1].IsNew)
	assert.True(t, plays[1].IsLivePerformance)
}

func TestKEXPProvider_FetchPlaylist_OnlyTrackPlays(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/shows/1234/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         1234,
			"start_time": "2026-01-15T06:00:00Z",
		})
	})
	mux.HandleFunc("/v2/plays/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"next":  nil,
			"count": 3,
			"results": []map[string]interface{}{
				{"id": 1, "play_type": "airbreak", "artist": ""},
				{"id": 2, "play_type": "stationid", "artist": ""},
				{"id": 3, "play_type": "trackplay", "artist": "The National", "airdate": "2026-01-15T06:00:00Z"},
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	plays, err := provider.FetchPlaylist("1234")

	require.NoError(t, err)
	assert.Len(t, plays, 1) // Only the trackplay
	assert.Equal(t, "The National", plays[0].ArtistName)
}

func TestKEXPProvider_FetchPlaylist_EmptyPlays(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/shows/9999/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         9999,
			"start_time": "2026-01-15T06:00:00Z",
		})
	})
	mux.HandleFunc("/v2/plays/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"next":    nil,
			"count":   0,
			"results": []map[string]interface{}{},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	plays, err := provider.FetchPlaylist("9999")

	require.NoError(t, err)
	assert.Empty(t, plays)
}

func TestKEXPProvider_FetchPlaylist_ShowNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/shows/404/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"detail":"Not found."}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	plays, err := provider.FetchPlaylist("404")

	require.NoError(t, err, "404 on show detail should return empty slice, not error")
	assert.Empty(t, plays)
}

func TestKEXPProvider_FetchPlaylist_Pagination(t *testing.T) {
	playsCallCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/shows/5678/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         5678,
			"start_time": "2026-01-15T06:00:00Z",
		})
	})

	var server *httptest.Server
	mux.HandleFunc("/v2/plays/", func(w http.ResponseWriter, r *http.Request) {
		playsCallCount++
		if playsCallCount == 1 {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"next":  fmt.Sprintf("%s/v2/plays/?cursor=page2", server.URL),
				"count": 3,
				"results": []map[string]interface{}{
					{
						"id":        1,
						"play_type": "trackplay",
						"airdate":   "2026-01-15T06:05:00Z",
						"artist":    "Radiohead",
						"song":      "Idioteque",
					},
					{
						"id":        2,
						"play_type": "trackplay",
						"airdate":   "2026-01-15T06:10:00Z",
						"artist":    "Deerhunter",
						"song":      "Desire Lines",
					},
				},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"next":  nil,
				"count": 3,
				"results": []map[string]interface{}{
					{
						"id":        3,
						"play_type": "trackplay",
						"airdate":   "2026-01-15T06:15:00Z",
						"artist":    "The National",
						"song":      "Bloodbuzz Ohio",
					},
				},
			})
		}
	})

	server = httptest.NewServer(mux)
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	plays, err := provider.FetchPlaylist("5678")

	require.NoError(t, err)
	assert.Len(t, plays, 3)
	assert.Equal(t, 2, playsCallCount, "should follow pagination cursor")

	// Positions should be sequential across pages.
	assert.Equal(t, 0, plays[0].Position)
	assert.Equal(t, "Radiohead", plays[0].ArtistName)
	assert.Equal(t, 1, plays[1].Position)
	assert.Equal(t, "Deerhunter", plays[1].ArtistName)
	assert.Equal(t, 2, plays[2].Position)
	assert.Equal(t, "The National", plays[2].ArtistName)
}

func TestKEXPProvider_HTTPError(t *testing.T) {
	mux := http.NewServeMux()
	// Hosts can succeed (non-fatal)
	mux.HandleFunc("/v2/hosts/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"next": nil, "count": 0, "results": []interface{}{},
		})
	})
	// Programs returns 500
	mux.HandleFunc("/v2/programs/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	_, err := provider.DiscoverShows()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

// =============================================================================
// UNIT TESTS: NilDB for new import/matching methods
// =============================================================================

func TestRadioService_NilDB_Import(t *testing.T) {
	svc := &RadioService{db: nil}
	assertNilDBError(t, func() error { _, err := svc.ImportStation(1, 7); return err })
	assertNilDBError(t, func() error { _, err := svc.FetchNewEpisodes(1); return err })
	assertNilDBError(t, func() error { _, err := svc.ImportEpisodePlaylist(1, "ext-1"); return err })
	assertNilDBError(t, func() error { _, err := svc.MatchPlays(1); return err })
	assertNilDBError(t, func() error { _, err := svc.DiscoverStationShows(1); return err })
	assertNilDBError(t, func() error { _, err := svc.ImportShowEpisodes(1, "2024-01-01", "2024-12-31"); return err })
}

func TestRadioMatchingEngine_NilDB(t *testing.T) {
	engine := NewRadioMatchingEngine(nil)

	_, err := engine.MatchPlaysForEpisode(1)
	assert.Error(t, err)
	assert.Equal(t, "database not initialized", err.Error())

	_, err = engine.MatchAllUnmatched()
	assert.Error(t, err)
	assert.Equal(t, "database not initialized", err.Error())
}

// =============================================================================
// INTEGRATION TESTS: Matching engine with real database
// =============================================================================

type RadioImportIntegrationTestSuite struct {
	suite.Suite
	testDB       *testutil.TestDatabase
	db           *gorm.DB
	radioService *RadioService
}

func (suite *RadioImportIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.radioService = &RadioService{db: suite.testDB.DB}
}

func (suite *RadioImportIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *RadioImportIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM radio_artist_affinity")
	_, _ = sqlDB.Exec("DELETE FROM radio_plays")
	_, _ = sqlDB.Exec("DELETE FROM radio_episodes")
	_, _ = sqlDB.Exec("DELETE FROM radio_shows")
	_, _ = sqlDB.Exec("DELETE FROM radio_stations")
	_, _ = sqlDB.Exec("DELETE FROM artist_aliases")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM labels")
}

func TestRadioImportIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(RadioImportIntegrationTestSuite))
}

// =============================================================================
// Matching engine integration tests
// =============================================================================

func (suite *RadioImportIntegrationTestSuite) TestMatchPlays_ExactNameMatch() {
	// Create station, show, episode, play
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")
	suite.createPlay(ep.ID, 0, "Radiohead")

	// Create the artist in our knowledge graph
	suite.createArtist("Radiohead")

	// Run matching
	result, err := suite.radioService.MatchPlays(ep.ID)

	suite.Require().NoError(err)
	suite.Equal(1, result.Total)
	suite.Equal(1, result.Matched)
	suite.Equal(0, result.Unmatched)

	// Verify play is now linked
	var play models.RadioPlay
	suite.db.Where("episode_id = ?", ep.ID).First(&play)
	suite.NotNil(play.ArtistID)
}

func (suite *RadioImportIntegrationTestSuite) TestMatchPlays_CaseInsensitiveMatch() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")
	suite.createPlay(ep.ID, 0, "RADIOHEAD") // uppercase in play

	suite.createArtist("Radiohead") // mixed case in DB

	result, err := suite.radioService.MatchPlays(ep.ID)

	suite.Require().NoError(err)
	suite.Equal(1, result.Matched)
}

func (suite *RadioImportIntegrationTestSuite) TestMatchPlays_AliasMatch() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")
	suite.createPlay(ep.ID, 0, "Thom Yorke") // Alias name

	// Create artist with alias
	artist := suite.createArtist("Radiohead")
	alias := &models.ArtistAlias{
		ArtistID: artist.ID,
		Alias:    "Thom Yorke",
	}
	suite.Require().NoError(suite.db.Create(alias).Error)

	result, err := suite.radioService.MatchPlays(ep.ID)

	suite.Require().NoError(err)
	suite.Equal(1, result.Matched)

	// Verify linked to the canonical artist
	var play models.RadioPlay
	suite.db.Where("episode_id = ?", ep.ID).First(&play)
	suite.NotNil(play.ArtistID)
	suite.Equal(artist.ID, *play.ArtistID)
}

func (suite *RadioImportIntegrationTestSuite) TestMatchPlays_Unmatched() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")
	suite.createPlay(ep.ID, 0, "Totally Unknown Band")

	result, err := suite.radioService.MatchPlays(ep.ID)

	suite.Require().NoError(err)
	suite.Equal(1, result.Total)
	suite.Equal(0, result.Matched)
	suite.Equal(1, result.Unmatched)

	// Verify play still has no artist_id
	var play models.RadioPlay
	suite.db.Where("episode_id = ?", ep.ID).First(&play)
	suite.Nil(play.ArtistID)
}

func (suite *RadioImportIntegrationTestSuite) TestMatchPlays_LabelMatch() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	labelName := "Sub Pop"
	play := &models.RadioPlay{
		EpisodeID:  ep.ID,
		Position:   0,
		ArtistName: "Fleet Foxes",
		LabelName:  &labelName,
	}
	suite.Require().NoError(suite.db.Create(play).Error)

	// Create artist and label in our graph
	suite.createArtist("Fleet Foxes")
	suite.createLabel("Sub Pop")

	result, err := suite.radioService.MatchPlays(ep.ID)

	suite.Require().NoError(err)
	suite.Equal(1, result.Matched) // Artist matched

	// Verify label also matched
	var updated models.RadioPlay
	suite.db.First(&updated, play.ID)
	suite.NotNil(updated.ArtistID)
	suite.NotNil(updated.LabelID)
}

func (suite *RadioImportIntegrationTestSuite) TestMatchPlays_ReleaseMatch() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	albumTitle := "Kid A"
	play := &models.RadioPlay{
		EpisodeID:  ep.ID,
		Position:   0,
		ArtistName: "Radiohead",
		AlbumTitle: &albumTitle,
	}
	suite.Require().NoError(suite.db.Create(play).Error)

	suite.createArtist("Radiohead")
	suite.createRelease("Kid A")

	result, err := suite.radioService.MatchPlays(ep.ID)

	suite.Require().NoError(err)
	suite.Equal(1, result.Matched)

	var updated models.RadioPlay
	suite.db.First(&updated, play.ID)
	suite.NotNil(updated.ArtistID)
	suite.NotNil(updated.ReleaseID)
}

func (suite *RadioImportIntegrationTestSuite) TestMatchPlays_SkipsAlreadyMatched() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	artist := suite.createArtist("Radiohead")

	// Create a play that's already matched
	play := &models.RadioPlay{
		EpisodeID:  ep.ID,
		Position:   0,
		ArtistName: "Radiohead",
		ArtistID:   &artist.ID,
	}
	suite.Require().NoError(suite.db.Create(play).Error)

	// MatchPlays only operates on unmatched plays (artist_id IS NULL)
	result, err := suite.radioService.MatchPlays(ep.ID)

	suite.Require().NoError(err)
	suite.Equal(0, result.Total) // Already matched play not included
}

func (suite *RadioImportIntegrationTestSuite) TestMatchPlays_MultiplePlaysMixed() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	suite.createPlay(ep.ID, 0, "Radiohead")
	suite.createPlay(ep.ID, 1, "Unknown Band XYZ")
	suite.createPlay(ep.ID, 2, "Deerhunter")

	suite.createArtist("Radiohead")
	suite.createArtist("Deerhunter")

	result, err := suite.radioService.MatchPlays(ep.ID)

	suite.Require().NoError(err)
	suite.Equal(3, result.Total)
	suite.Equal(2, result.Matched)
	suite.Equal(1, result.Unmatched)
}

// =============================================================================
// Import pipeline integration tests
// =============================================================================

func (suite *RadioImportIntegrationTestSuite) TestImportStation_Success() {
	// Create a station with KEXP source pointing at mock server
	mux := http.NewServeMux()
	var server *httptest.Server

	// Mock hosts
	mux.HandleFunc("/v2/hosts/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"next": nil, "count": 1,
			"results": []map[string]interface{}{
				{"id": 1, "name": "John Richards"},
			},
		})
	})

	// Mock programs
	mux.HandleFunc("/v2/programs/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"next": nil, "count": 1,
			"results": []map[string]interface{}{
				{"id": 42, "name": "The Morning Show", "host_ids": []int{1}},
			},
		})
	})

	// Mock shows (episodes) — handles both list and detail-by-ID requests.
	mux.HandleFunc("/v2/shows/", func(w http.ResponseWriter, r *http.Request) {
		// Detail-by-ID: /v2/shows/100/ used by FetchPlaylist.
		if r.URL.Path == "/v2/shows/100/" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":           100,
				"program_id":   42,
				"program_name": "The Morning Show",
				"start_time":   "2026-01-15T06:00:00-08:00",
				"end_time":     "2026-01-15T10:00:00-08:00",
			})
			return
		}
		// List: /v2/shows/?program_id=... used by FetchNewEpisodes.
		json.NewEncoder(w).Encode(map[string]interface{}{
			"next": nil, "count": 1,
			"results": []map[string]interface{}{
				{
					"id":           100,
					"program_id":   42,
					"program_name": "The Morning Show",
					"start_time":   "2026-01-15T06:00:00-08:00",
					"end_time":     "2026-01-15T10:00:00-08:00",
				},
			},
		})
	})

	// Mock plays
	mux.HandleFunc("/v2/plays/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"next": nil, "count": 2,
			"results": []map[string]interface{}{
				{
					"id":        1,
					"play_type": "trackplay",
					"artist":    "Radiohead",
					"song":      "Idioteque",
					"album":     "Kid A",
					"airdate":   "2026-01-15T06:05:00-08:00",
				},
				{
					"id":        2,
					"play_type": "trackplay",
					"artist":    "Deerhunter",
					"song":      "Desire Lines",
					"airdate":   "2026-01-15T06:10:00-08:00",
				},
			},
		})
	})

	server = httptest.NewServer(mux)
	defer server.Close()

	// Create station with a temporary provider override
	source := models.PlaylistSourceKEXP
	stationResp, err := suite.radioService.CreateStation(&contracts.CreateRadioStationRequest{
		Name:           "KEXP",
		BroadcastType:  models.BroadcastTypeBoth,
		PlaylistSource: &source,
	})
	suite.Require().NoError(err)

	// Create a mock provider and test the import pipeline directly
	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	// Run the import directly using the provider (since getProvider returns the real KEXP URL)
	result := suite.runImportWithProvider(stationResp.ID, 30, provider)

	suite.Equal(1, result.ShowsDiscovered)
	suite.Equal(1, result.EpisodesImported)
	suite.Equal(2, result.PlaysImported)
	suite.Empty(result.Errors)
}

func (suite *RadioImportIntegrationTestSuite) TestImportStation_NoPlaylistSource() {
	stationResp, err := suite.radioService.CreateStation(&contracts.CreateRadioStationRequest{
		Name:          "No Source",
		BroadcastType: models.BroadcastTypeBoth,
	})
	suite.Require().NoError(err)

	_, err = suite.radioService.ImportStation(stationResp.ID, 7)
	suite.Error(err)
	suite.Contains(err.Error(), "no playlist source configured")
}

func (suite *RadioImportIntegrationTestSuite) TestImportStation_StationNotFound() {
	_, err := suite.radioService.ImportStation(99999, 7)
	suite.Error(err)
	suite.Contains(err.Error(), "station not found")
}

func (suite *RadioImportIntegrationTestSuite) TestUpsertRadioShow_CreateNew() {
	station := suite.createStation("KEXP")

	importShow := RadioShowImport{
		ExternalID: "42",
		Name:       "The Morning Show",
		HostName:   stringPtr("John Richards"),
	}

	showID, err := suite.radioService.upsertRadioShow(station.ID, importShow)
	suite.Require().NoError(err)
	suite.NotZero(showID)

	// Verify show was created
	var show models.RadioShow
	suite.db.First(&show, showID)
	suite.Equal("The Morning Show", show.Name)
	suite.Equal("John Richards", *show.HostName)
	suite.Equal("42", *show.ExternalID)
}

func (suite *RadioImportIntegrationTestSuite) TestUpsertRadioShow_UpdateExisting() {
	station := suite.createStation("KEXP")

	// Create initial show
	extID := "42"
	show := &models.RadioShow{
		StationID:  station.ID,
		Name:       "Old Name",
		Slug:       "old-name",
		ExternalID: &extID,
	}
	suite.Require().NoError(suite.db.Create(show).Error)

	// Upsert with new name
	importShow := RadioShowImport{
		ExternalID: "42",
		Name:       "New Name",
		HostName:   stringPtr("New Host"),
	}

	showID, err := suite.radioService.upsertRadioShow(station.ID, importShow)
	suite.Require().NoError(err)
	suite.Equal(show.ID, showID)

	// Verify update
	var updated models.RadioShow
	suite.db.First(&updated, showID)
	suite.Equal("New Name", updated.Name)
	suite.Equal("New Host", *updated.HostName)
}

func (suite *RadioImportIntegrationTestSuite) TestImportEpisode_DeduplicatesByExternalID() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")

	// Create a mock provider
	mockProvider := &mockPlaylistProvider{
		fetchPlaylistFn: func(epExtID string) ([]RadioPlayImport, error) {
			return []RadioPlayImport{
				{Position: 0, ArtistName: "Radiohead"},
			}, nil
		},
	}

	ep := RadioEpisodeImport{
		ExternalID:     "100",
		ShowExternalID: "42",
		AirDate:        "2026-01-15",
	}

	// First import
	result1, err := suite.radioService.importEpisode(show.ID, ep, mockProvider)
	suite.Require().NoError(err)
	suite.Equal(1, result1.PlaysImported)

	// Second import — should be skipped (deduplication)
	result2, err := suite.radioService.importEpisode(show.ID, ep, mockProvider)
	suite.Require().NoError(err)
	suite.Equal(0, result2.PlaysImported)

	// Verify only one episode exists
	var count int64
	suite.db.Model(&models.RadioEpisode{}).Where("show_id = ?", show.ID).Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *RadioImportIntegrationTestSuite) TestImportPlays_BatchInsert() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	plays := []RadioPlayImport{
		{Position: 0, ArtistName: "Radiohead", IsNew: true},
		{Position: 1, ArtistName: "Deerhunter", IsLivePerformance: true},
		{Position: 2, ArtistName: "Sonic Youth"},
	}

	count, err := suite.radioService.importPlays(ep.ID, plays)

	suite.Require().NoError(err)
	suite.Equal(3, count)

	// Verify plays in DB
	var dbPlays []models.RadioPlay
	suite.db.Where("episode_id = ?", ep.ID).Order("position ASC").Find(&dbPlays)
	suite.Len(dbPlays, 3)
	suite.Equal("Radiohead", dbPlays[0].ArtistName)
	suite.True(dbPlays[0].IsNew)
	suite.Equal("Deerhunter", dbPlays[1].ArtistName)
	suite.True(dbPlays[1].IsLivePerformance)
	suite.Equal("Sonic Youth", dbPlays[2].ArtistName)
}

func (suite *RadioImportIntegrationTestSuite) TestImportPlays_Empty() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	count, err := suite.radioService.importPlays(ep.ID, []RadioPlayImport{})

	suite.Require().NoError(err)
	suite.Equal(0, count)
}

func (suite *RadioImportIntegrationTestSuite) TestMatchPlays_LabelCaseInsensitive() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	labelName := "sub pop" // lowercase
	play := &models.RadioPlay{
		EpisodeID:  ep.ID,
		Position:   0,
		ArtistName: "Fleet Foxes",
		LabelName:  &labelName,
	}
	suite.Require().NoError(suite.db.Create(play).Error)

	suite.createArtist("Fleet Foxes")
	suite.createLabel("Sub Pop") // mixed case in DB

	result, err := suite.radioService.MatchPlays(ep.ID)

	suite.Require().NoError(err)
	suite.Equal(1, result.Matched)

	var updated models.RadioPlay
	suite.db.First(&updated, play.ID)
	suite.NotNil(updated.LabelID) // Case-insensitive match
}

func (suite *RadioImportIntegrationTestSuite) TestGetProvider_Unsupported() {
	_, err := suite.radioService.getProvider("unsupported_source")
	suite.Error(err)
	suite.Contains(err.Error(), "unsupported playlist source")
}

func (suite *RadioImportIntegrationTestSuite) TestGetProvider_KEXP() {
	provider, err := suite.radioService.getProvider(models.PlaylistSourceKEXP)
	suite.Require().NoError(err)
	suite.NotNil(provider)
	closeProvider(provider)
}

// =============================================================================
// Test helpers
// =============================================================================

func (suite *RadioImportIntegrationTestSuite) createStation(name string) *contracts.RadioStationDetailResponse {
	resp, err := suite.radioService.CreateStation(&contracts.CreateRadioStationRequest{
		Name:          name,
		BroadcastType: models.BroadcastTypeBoth,
	})
	suite.Require().NoError(err)
	return resp
}

func (suite *RadioImportIntegrationTestSuite) createShow(stationID uint, name string) *contracts.RadioShowDetailResponse {
	resp, err := suite.radioService.CreateShow(stationID, &contracts.CreateRadioShowRequest{
		Name: name,
	})
	suite.Require().NoError(err)
	return resp
}

func (suite *RadioImportIntegrationTestSuite) createEpisode(showID uint, airDate string) *models.RadioEpisode {
	ep := &models.RadioEpisode{
		ShowID:  showID,
		AirDate: airDate,
	}
	err := suite.db.Create(ep).Error
	suite.Require().NoError(err)
	return ep
}

func (suite *RadioImportIntegrationTestSuite) createPlay(episodeID uint, position int, artistName string) *models.RadioPlay {
	play := &models.RadioPlay{
		EpisodeID:  episodeID,
		Position:   position,
		ArtistName: artistName,
	}
	err := suite.db.Create(play).Error
	suite.Require().NoError(err)
	return play
}

func (suite *RadioImportIntegrationTestSuite) createArtist(name string) *models.Artist {
	slug := utils.GenerateArtistSlug(name)
	artist := &models.Artist{Name: name, Slug: &slug}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *RadioImportIntegrationTestSuite) createRelease(title string) *models.Release {
	release := &models.Release{Title: title}
	err := suite.db.Create(release).Error
	suite.Require().NoError(err)
	return release
}

func (suite *RadioImportIntegrationTestSuite) createLabel(name string) *models.Label {
	label := &models.Label{Name: name, Status: models.LabelStatusActive}
	err := suite.db.Create(label).Error
	suite.Require().NoError(err)
	return label
}

// runImportWithProvider runs the import pipeline with a specific provider (bypassing getProvider).
func (suite *RadioImportIntegrationTestSuite) runImportWithProvider(stationID uint, backfillDays int, provider RadioPlaylistProvider) *contracts.RadioImportResult {
	var station models.RadioStation
	suite.Require().NoError(suite.db.First(&station, stationID).Error)

	result := &contracts.RadioImportResult{}

	importedShows, err := provider.DiscoverShows()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("discover shows: %v", err))
		return result
	}

	showMap := make(map[string]uint)
	for _, importShow := range importedShows {
		showID, err := suite.radioService.upsertRadioShow(stationID, importShow)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("upsert show: %v", err))
			continue
		}
		showMap[importShow.ExternalID] = showID
		result.ShowsDiscovered++
	}

	since := time.Now().AddDate(0, 0, -backfillDays)
	for extID, showID := range showMap {
		episodes, err := provider.FetchNewEpisodes(extID, since)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("fetch episodes: %v", err))
			continue
		}

		for _, ep := range episodes {
			epResult, err := suite.radioService.importEpisode(showID, ep, provider)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("import episode: %v", err))
				continue
			}
			result.EpisodesImported++
			result.PlaysImported += epResult.PlaysImported
			result.PlaysMatched += epResult.PlaysMatched
		}
	}

	return result
}

// =============================================================================
// Mock provider
// =============================================================================

type mockPlaylistProvider struct {
	discoverShowsFn    func() ([]RadioShowImport, error)
	fetchNewEpisodesFn func(showExternalID string, since time.Time) ([]RadioEpisodeImport, error)
	fetchPlaylistFn    func(episodeExternalID string) ([]RadioPlayImport, error)
}

func (m *mockPlaylistProvider) DiscoverShows() ([]RadioShowImport, error) {
	if m.discoverShowsFn != nil {
		return m.discoverShowsFn()
	}
	return nil, nil
}

func (m *mockPlaylistProvider) FetchNewEpisodes(showExternalID string, since time.Time) ([]RadioEpisodeImport, error) {
	if m.fetchNewEpisodesFn != nil {
		return m.fetchNewEpisodesFn(showExternalID, since)
	}
	return nil, nil
}

func (m *mockPlaylistProvider) FetchPlaylist(episodeExternalID string) ([]RadioPlayImport, error) {
	if m.fetchPlaylistFn != nil {
		return m.fetchPlaylistFn(episodeExternalID)
	}
	return nil, nil
}
