package catalog

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
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
		{"12", 0},   // too short
		{"0000", 0}, // out of range
		{"9999", 0}, // out of range
		{"1899", 0}, // out of range (< 1900)
		{"2101", 0}, // out of range (> 2100)
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
		Labels:                 []string{"Parlophone"},
		ReleaseDate:            "2000-10-02",
		RotationStatus:         "heavy",
		IsLive:                 false,
		IsRequest:              true,
		Comment:                "Classic album opener",
		ArtistIDs:              []string{"a74b1b7f-71a5-4011-9441-d0b5e4122711"},
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

// TestParseKEXPEpisode_NoEndTime exercises the list-endpoint shape: KEXP's
// /v2/shows/ list response omits both `end_time` and `archive_url`, so the
// parser's defensive guards must leave DurationMinutes and ArchiveURL nil
// without panicking (PSY-813).
func TestParseKEXPEpisode_NoEndTime(t *testing.T) {
	show := kexpShow{
		ID:        9999,
		StartTime: "2026-01-15T06:00:00-08:00",
	}

	ep := parseKEXPEpisode(show, "1")

	assert.Equal(t, "9999", ep.ExternalID)
	assert.Equal(t, "2026-01-15", ep.AirDate)
	assert.Nil(t, ep.DurationMinutes)
	assert.Nil(t, ep.ArchiveURL,
		"PSY-813: list endpoint omits archive_url, parser must leave ArchiveURL nil")
}

// =============================================================================
// UNIT TESTS: KEXP provider with mock HTTP server
// =============================================================================

func TestKEXPProvider_DiscoverShows(t *testing.T) {
	mux := http.NewServeMux()

	// PSY-509: KEXP's /v2/programs/ endpoint does NOT carry host info on
	// programs. DiscoverShows derives host attribution from /v2/shows/
	// (broadcast level), where each broadcast has a resolved host_names
	// array. The shows handler must come BEFORE the programs handler so the
	// most-specific path matches first in net/http's mux.
	mux.HandleFunc("/v2/shows/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next":  nil,
			"count": 3,
			"results": []map[string]interface{}{
				{
					"id":         1001,
					"program":    42,
					"host_names": []string{"John Richards"},
					"start_time": "2026-04-22T06:00:00-07:00",
				},
				{
					"id":         1002,
					"program":    43,
					"host_names": []string{"Cheryl Waters"},
					"start_time": "2026-04-22T10:00:00-07:00",
				},
				// Older broadcast for program 42 with a different host —
				// should be ignored because we already have the most-recent
				// attribution.
				{
					"id":         900,
					"program":    42,
					"host_names": []string{"Substitute DJ"},
					"start_time": "2026-04-21T06:00:00-07:00",
				},
			},
		})
	})

	// Mock programs endpoint — note: the real API does NOT return
	// host_ids/host_names on programs, so the test omits those fields.
	mux.HandleFunc("/v2/programs/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next":  nil,
			"count": 2,
			"results": []map[string]interface{}{
				{
					"id":          42,
					"name":        "The Morning Show",
					"description": "Wake up with KEXP",
					"image_uri":   "https://kexp.org/morning.jpg",
					"is_active":   true,
				},
				{
					"id":        43,
					"name":      "The Midday Show",
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
	require.NotNil(t, shows[0].HostName, "PSY-509: host_name must be populated from broadcast-derived map")
	assert.Equal(t, "John Richards", *shows[0].HostName)
	// PSY-405: DiscoverShows no longer fabricates an archive URL — KEXP's
	// per-show URL casing isn't derivable from the API name.
	assert.Nil(t, shows[0].ArchiveURL)

	assert.Equal(t, "43", shows[1].ExternalID)
	assert.Equal(t, "The Midday Show", shows[1].Name)
	require.NotNil(t, shows[1].HostName)
	assert.Equal(t, "Cheryl Waters", *shows[1].HostName)
	assert.Nil(t, shows[1].ArchiveURL)
}

func TestKEXPProvider_DiscoverShows_Pagination(t *testing.T) {
	callCount := 0
	mux := http.NewServeMux()

	// PSY-509: empty broadcast slice — exercise the warn-log "empty map"
	// branch alongside paginated programs.
	mux.HandleFunc("/v2/shows/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next": nil, "count": 0, "results": []interface{}{},
		})
	})

	var server *httptest.Server
	mux.HandleFunc("/v2/programs/", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"next":  fmt.Sprintf("%s/v2/programs/?offset=1", server.URL),
				"count": 2,
				"results": []map[string]interface{}{
					{"id": 1, "name": "Show One"},
				},
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"next":  nil,
				"count": 2,
				"results": []map[string]interface{}{
					{"id": 2, "name": "Show Two"},
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
	// With an empty broadcast slice the host map is empty — programs come
	// through with host_name nil, which is the documented graceful
	// degradation when the broadcast endpoint can't be queried.
	assert.Nil(t, shows[0].HostName)
	assert.Nil(t, shows[1].HostName)
}

// PSY-813: KEXP's /v2/shows/ LIST endpoint does NOT carry `end_time` or
// `archive_url` on its results — only the /v2/shows/{id}/ DETAIL endpoint
// does. The previous fixture set both fields and asserted DurationMinutes was
// populated, which contradicted production behavior. The corrected fixture
// matches the real API and the assertions document the contract: list-imported
// episodes always land with DurationMinutes == nil and ArchiveURL == nil.
func TestKEXPProvider_FetchNewEpisodes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/shows/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next":  nil,
			"count": 1,
			"results": []map[string]interface{}{
				{
					"id":           5678,
					"program":      42,
					"program_name": "The Morning Show",
					"start_time":   "2026-01-15T06:00:00-08:00",
					// end_time and archive_url intentionally omitted — the
					// real /v2/shows/ list response does not include them.
				},
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	episodes, err := provider.FetchNewEpisodes("42", since, time.Time{})

	require.NoError(t, err)
	assert.Len(t, episodes, 1)
	assert.Equal(t, "5678", episodes[0].ExternalID)
	assert.Equal(t, "42", episodes[0].ShowExternalID)
	assert.Equal(t, "2026-01-15", episodes[0].AirDate)
	// PSY-813: list-endpoint contract — duration + archive must be nil because
	// the source payload omits end_time / archive_url. parseKEXPEpisode's
	// defensive guards make this graceful; FetchPlaylist's detail-endpoint
	// roundtrip is where these fields would be populated if backfilled.
	assert.Nil(t, episodes[0].DurationMinutes,
		"list endpoint omits end_time, so DurationMinutes must be nil")
	assert.Nil(t, episodes[0].ArchiveURL,
		"list endpoint omits archive_url, so ArchiveURL must be nil")
}

func TestKEXPProvider_FetchNewEpisodes_FiltersByProgram(t *testing.T) {
	// The KEXP API ignores the program_id query param and returns broadcasts
	// for ALL programs, so the provider filters client-side on each broadcast's
	// `program` field. Regression: kexpShow was tagged json:"program_id" while
	// the real field is `program`, so the filter dropped every broadcast (0
	// episodes imported for every KEXP program).
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/shows/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next": nil, "count": 3,
			"results": []map[string]interface{}{
				{"id": 1, "program": 42, "program_name": "Wanted", "start_time": "2026-01-15T06:00:00-08:00"},
				{"id": 2, "program": 99, "program_name": "Other", "start_time": "2026-01-15T08:00:00-08:00"},
				{"id": 3, "program": 42, "program_name": "Wanted", "start_time": "2026-01-16T06:00:00-08:00"},
			},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	episodes, err := provider.FetchNewEpisodes("42", since, time.Time{})
	require.NoError(t, err)
	require.Len(t, episodes, 2, "only program 42's broadcasts should pass the client-side filter")
	assert.Equal(t, "1", episodes[0].ExternalID)
	assert.Equal(t, "3", episodes[1].ExternalID)
}

func TestKEXPProvider_FetchPlaylist(t *testing.T) {
	var playsRequestQuery string

	mux := http.NewServeMux()
	// Show detail endpoint -- returns start_time AND end_time so the provider
	// should use the actual broadcast window (4 hours) instead of the fallback.
	mux.HandleFunc("/v2/shows/5678/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           5678,
			"program":      42,
			"program_name": "The Morning Show",
			"start_time":   "2026-01-15T06:00:00-08:00",
			"end_time":     "2026-01-15T10:00:00-08:00",
		})
	})
	mux.HandleFunc("/v2/plays/", func(w http.ResponseWriter, r *http.Request) {
		playsRequestQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next":  nil,
			"count": 3,
			"results": []map[string]interface{}{
				{
					"id":              1,
					"play_type":       "trackplay",
					"airdate":         "2026-01-15T06:05:00-08:00",
					"artist":          "Radiohead",
					"song":            "Everything In Its Right Place",
					"album":           "Kid A",
					"labels":          []string{"Parlophone"},
					"release_date":    "2000-10-02",
					"rotation_status": "library",
					"is_live":         false,
					"is_request":      false,
					"artist_ids":      []string{"a74b1b7f-71a5-4011-9441-d0b5e4122711"},
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
	// airdate_before should match the show end_time (UTC). The broadcast
	// ends at 2026-01-15T10:00:00-08:00 which is 2026-01-15T18:00:00Z.
	assert.Contains(t, playsRequestQuery, "airdate_before=2026-01-15T18:00:00Z")

	// First play
	assert.Equal(t, 0, plays[0].Position)
	assert.Equal(t, "Radiohead", plays[0].ArtistName)
	assert.Equal(t, "Everything In Its Right Place", *plays[0].TrackTitle)
	assert.Equal(t, "Kid A", *plays[0].AlbumTitle)
	assert.Equal(t, "Parlophone", *plays[0].LabelName)
	assert.Equal(t, 2000, *plays[0].ReleaseYear)
	assert.Equal(t, "library", *plays[0].RotationStatus)
	assert.Equal(t, "a74b1b7f-71a5-4011-9441-d0b5e4122711", *plays[0].MusicBrainzArtistID)

	// Second play
	assert.Equal(t, 1, plays[1].Position)
	assert.Equal(t, "Deerhunter", plays[1].ArtistName)
	assert.True(t, plays[1].IsLivePerformance)
}

func TestKEXPProvider_FetchPlaylist_NoEndTimeFallback(t *testing.T) {
	var playsRequestQuery string

	mux := http.NewServeMux()
	// Show detail endpoint -- no end_time, so provider should fall back to
	// kexpPlaylistWindowFallback (5 hours).
	mux.HandleFunc("/v2/shows/7777/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         7777,
			"start_time": "2026-01-15T14:00:00Z",
		})
	})
	mux.HandleFunc("/v2/plays/", func(w http.ResponseWriter, r *http.Request) {
		playsRequestQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next":  nil,
			"count": 1,
			"results": []map[string]interface{}{
				{
					"id":        1,
					"play_type": "trackplay",
					"airdate":   "2026-01-15T14:05:00Z",
					"artist":    "Yo La Tengo",
					"song":      "Tom Courtenay",
				},
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	plays, err := provider.FetchPlaylist("7777")

	require.NoError(t, err)
	assert.Len(t, plays, 1)
	assert.Equal(t, "Yo La Tengo", plays[0].ArtistName)

	// Without end_time, should fall back to start_time + 5h = 2026-01-15T19:00:00Z
	assert.Contains(t, playsRequestQuery, "airdate_after=2026-01-15T14:00:00Z")
	assert.Contains(t, playsRequestQuery, "airdate_before=2026-01-15T19:00:00Z")
}

func TestKEXPProvider_FetchPlaylist_OnlyTrackPlays(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/shows/1234/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         1234,
			"start_time": "2026-01-15T06:00:00Z",
		})
	})
	mux.HandleFunc("/v2/plays/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
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
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         9999,
			"start_time": "2026-01-15T06:00:00Z",
		})
	})
	mux.HandleFunc("/v2/plays/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
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
		_, _ = w.Write([]byte(`{"detail":"Not found."}`))
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
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         5678,
			"start_time": "2026-01-15T06:00:00Z",
		})
	})

	var server *httptest.Server
	mux.HandleFunc("/v2/plays/", func(w http.ResponseWriter, r *http.Request) {
		playsCallCount++
		if playsCallCount == 1 {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
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
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
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
	// PSY-509: shows endpoint (used to build the host map) succeeds —
	// host-map failures are non-fatal so we must isolate the programs error.
	mux.HandleFunc("/v2/shows/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next": nil, "count": 0, "results": []interface{}{},
		})
	})
	// Programs returns 500
	mux.HandleFunc("/v2/programs/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	_, err := provider.DiscoverShows()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

// TestKEXPProvider_DiscoverShows_HostMapNonFatal asserts that when the
// /v2/shows/ host-map fetch fails, DiscoverShows still returns programs
// successfully (just with host_name nil). This is the regression-guard
// for PSY-509 in the failure direction: the bug was that programs were
// returned without host_name even when the live API was healthy. We must
// also make sure programs ARE returned when the host-map endpoint itself
// errors out — host attribution is best-effort, not load-bearing.
func TestKEXPProvider_DiscoverShows_HostMapNonFatal(t *testing.T) {
	mux := http.NewServeMux()
	// /v2/shows/ returns 500 — host map fetch fails.
	mux.HandleFunc("/v2/shows/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream broken"))
	})
	// Programs endpoint healthy.
	mux.HandleFunc("/v2/programs/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next": nil, "count": 1,
			"results": []map[string]interface{}{
				{"id": 99, "name": "Show With No Host Yet"},
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	shows, err := provider.DiscoverShows()
	require.NoError(t, err, "host-map fetch failure must NOT fail DiscoverShows")
	assert.Len(t, shows, 1)
	assert.Equal(t, "Show With No Host Yet", shows[0].Name)
	assert.Nil(t, shows[0].HostName, "host_name nil is the graceful-degradation outcome")
}

// TestKEXPProvider_DiscoverShows_HostMappingIntegration is the PSY-509
// regression test: it asserts that a program with at least one matching
// recent broadcast (carrying host_names) results in a populated
// RadioShowImport.HostName. It also covers the multi-host join, the
// empty-string filter, and the "first broadcast wins per program"
// ordering invariant.
func TestKEXPProvider_DiscoverShows_HostMappingIntegration(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/v2/shows/", func(w http.ResponseWriter, r *http.Request) {
		// Results are returned in -start_time order, matching how the
		// real API responds when ordering=-start_time is requested.
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next": nil, "count": 6,
			"results": []map[string]interface{}{
				// Most recent: program 100 hosted by two co-DJs.
				{
					"id":         5001,
					"program":    100,
					"host_names": []string{"Albina Cabrera", "Goyri"},
					"start_time": "2026-04-22T21:00:00-07:00",
				},
				// Program 200 — single host.
				{
					"id":         5000,
					"program":    200,
					"host_names": []string{"Larry Mizell, Jr."},
					"start_time": "2026-04-22T14:00:00-07:00",
				},
				// Older broadcast for program 100 with a different host —
				// must NOT overwrite the first-seen attribution.
				{
					"id":         4999,
					"program":    100,
					"host_names": []string{"Substitute DJ"},
					"start_time": "2026-04-21T21:00:00-07:00",
				},
				// Program 300 with empty-string host (overnight automation).
				{
					"id":         4998,
					"program":    300,
					"host_names": []string{""},
					"start_time": "2026-04-21T03:00:00-07:00",
				},
				// Program 400 has no host_names field at all.
				{
					"id":         4997,
					"program":    400,
					"start_time": "2026-04-21T01:00:00-07:00",
				},
				// Program 500 with mixed empty + valid — the filter keeps
				// only the non-empty entries.
				{
					"id":         4996,
					"program":    500,
					"host_names": []string{"", "Cheryl Waters", ""},
					"start_time": "2026-04-20T10:00:00-07:00",
				},
			},
		})
	})

	mux.HandleFunc("/v2/programs/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next": nil, "count": 5,
			"results": []map[string]interface{}{
				{"id": 100, "name": "El Sonido"},
				{"id": 200, "name": "The Afternoon Show"},
				{"id": 300, "name": "Overnight Automation"},
				{"id": 400, "name": "No Hosts Listed"},
				{"id": 500, "name": "The Midday Show"},
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	shows, err := provider.DiscoverShows()
	require.NoError(t, err)
	require.Len(t, shows, 5)

	byID := map[string]RadioShowImport{}
	for _, s := range shows {
		byID[s.ExternalID] = s
	}

	// Multi-host program — joined with ", ".
	require.NotNil(t, byID["100"].HostName, "multi-host program must have HostName")
	assert.Equal(t, "Albina Cabrera, Goyri", *byID["100"].HostName,
		"multiple host_names must be joined with ', ' and reflect the most recent broadcast")

	// Single-host program.
	require.NotNil(t, byID["200"].HostName)
	assert.Equal(t, "Larry Mizell, Jr.", *byID["200"].HostName)

	// Empty-string-only host_names → no attribution.
	assert.Nil(t, byID["300"].HostName,
		"all-empty host_names must filter out and leave HostName nil")

	// Missing host_names entirely → no attribution.
	assert.Nil(t, byID["400"].HostName,
		"missing host_names array must leave HostName nil")

	// Mixed empty + valid → only non-empty kept.
	require.NotNil(t, byID["500"].HostName)
	assert.Equal(t, "Cheryl Waters", *byID["500"].HostName,
		"empty-string host entries must be filtered before joining")
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
	var play catalogm.RadioPlay
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
	alias := &catalogm.ArtistAlias{
		ArtistID: artist.ID,
		Alias:    "Thom Yorke",
	}
	suite.Require().NoError(suite.db.Create(alias).Error)

	result, err := suite.radioService.MatchPlays(ep.ID)

	suite.Require().NoError(err)
	suite.Equal(1, result.Matched)

	// Verify linked to the canonical artist
	var play catalogm.RadioPlay
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
	var play catalogm.RadioPlay
	suite.db.Where("episode_id = ?", ep.ID).First(&play)
	suite.Nil(play.ArtistID)
}

func (suite *RadioImportIntegrationTestSuite) TestMatchPlays_LabelMatch() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	labelName := "Sub Pop"
	play := &catalogm.RadioPlay{
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
	var updated catalogm.RadioPlay
	suite.db.First(&updated, play.ID)
	suite.NotNil(updated.ArtistID)
	suite.NotNil(updated.LabelID)
}

func (suite *RadioImportIntegrationTestSuite) TestMatchPlays_ReleaseMatch() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	albumTitle := "Kid A"
	play := &catalogm.RadioPlay{
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

	var updated catalogm.RadioPlay
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
	play := &catalogm.RadioPlay{
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

	// Mock programs
	mux.HandleFunc("/v2/programs/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next": nil, "count": 1,
			"results": []map[string]interface{}{
				{"id": 42, "name": "The Morning Show"},
			},
		})
	})

	// Mock shows (episodes) — handles three callers:
	//   1. PSY-509 host map: /v2/shows/?ordering=-start_time — must include
	//      a `host_names` array on each result so DiscoverShows can attach
	//      the host_name to program 42.
	//   2. Detail-by-ID: /v2/shows/100/ used by FetchPlaylist.
	//   3. List with program_id filter: /v2/shows/?program_id=... used by
	//      FetchNewEpisodes.
	mux.HandleFunc("/v2/shows/", func(w http.ResponseWriter, r *http.Request) {
		// Detail-by-ID: /v2/shows/100/ used by FetchPlaylist.
		if r.URL.Path == "/v2/shows/100/" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":           100,
				"program":      42,
				"program_name": "The Morning Show",
				"start_time":   "2026-01-15T06:00:00-08:00",
				"end_time":     "2026-01-15T10:00:00-08:00",
			})
			return
		}
		// List endpoint serves both the PSY-509 host-map fetch and the
		// FetchNewEpisodes filter. Both read the same `program` int field
		// (alongside `host_names`, which only the host map uses).
		//
		// PSY-813: the real /v2/shows/ list response does NOT include
		// `end_time` or `archive_url` — only the detail endpoint above does.
		// Omitting them here keeps the fixture aligned with production behavior.
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"next": nil, "count": 1,
			"results": []map[string]interface{}{
				{
					"id":           100,
					"program":      42,
					"program_name": "The Morning Show",
					"host_names":   []string{"John Richards"},
					"start_time":   "2026-01-15T06:00:00-08:00",
				},
			},
		})
	})

	// Mock plays
	mux.HandleFunc("/v2/plays/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
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
	source := catalogm.PlaylistSourceKEXP
	stationResp, err := suite.radioService.CreateStation(&contracts.CreateRadioStationRequest{
		Name:           "KEXP",
		BroadcastType:  catalogm.BroadcastTypeBoth,
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
		BroadcastType: catalogm.BroadcastTypeBoth,
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

	showID, _, err := suite.radioService.upsertRadioShow(station.ID, importShow)
	suite.Require().NoError(err)
	suite.NotZero(showID)

	// Verify show was created
	var show catalogm.RadioShow
	suite.db.First(&show, showID)
	suite.Equal("The Morning Show", show.Name)
	suite.Equal("John Richards", *show.HostName)
	suite.Equal("42", *show.ExternalID)
}

func (suite *RadioImportIntegrationTestSuite) TestUpsertRadioShow_PreservesCuratedData() {
	station := suite.createStation("KEXP")

	// Create initial show with curated data
	extID := "42"
	curatedDesc := "Curated description by admin"
	curatedHost := "Kennady Quille"
	show := &catalogm.RadioShow{
		StationID:   station.ID,
		Name:        "Audioasis",
		Slug:        "audioasis",
		HostName:    &curatedHost,
		Description: &curatedDesc,
		ExternalID:  &extID,
	}
	suite.Require().NoError(suite.db.Create(show).Error)

	// Upsert with different API data — curated fields should NOT be overwritten
	apiDesc := "Short API description"
	importShow := RadioShowImport{
		ExternalID:  "42",
		Name:        "Audioasis (API)",
		HostName:    stringPtr("Cheryl Waters"),
		Description: &apiDesc,
		ImageURL:    stringPtr("https://example.com/image.jpg"),
	}

	showID, _, err := suite.radioService.upsertRadioShow(station.ID, importShow)
	suite.Require().NoError(err)
	suite.Equal(show.ID, showID)

	// Verify curated values are preserved
	var updated catalogm.RadioShow
	suite.db.First(&updated, showID)
	suite.Equal("Audioasis", updated.Name)                            // kept curated name
	suite.Equal("Kennady Quille", *updated.HostName)                  // kept curated host
	suite.Equal("Curated description by admin", *updated.Description) // kept curated description
	// ImageURL was NULL, so it gets filled from import data
	suite.Require().NotNil(updated.ImageURL)
	suite.Equal("https://example.com/image.jpg", *updated.ImageURL)
}

func (suite *RadioImportIntegrationTestSuite) TestUpsertRadioShow_FillsEmptyFields() {
	station := suite.createStation("KEXP")

	// Create initial show with minimal data (no host, no description)
	extID := "42"
	show := &catalogm.RadioShow{
		StationID:  station.ID,
		Name:       "New Show",
		Slug:       "new-show",
		ExternalID: &extID,
	}
	suite.Require().NoError(suite.db.Create(show).Error)

	// Upsert with API data — empty fields should be populated
	importShow := RadioShowImport{
		ExternalID:  "42",
		Name:        "New Show (API)",
		HostName:    stringPtr("DJ Host"),
		Description: stringPtr("A great show"),
		ArchiveURL:  stringPtr("https://example.com/archive"),
	}

	showID, _, err := suite.radioService.upsertRadioShow(station.ID, importShow)
	suite.Require().NoError(err)
	suite.Equal(show.ID, showID)

	var updated catalogm.RadioShow
	suite.db.First(&updated, showID)
	suite.Equal("New Show", updated.Name)                           // name was non-empty, kept
	suite.Equal("DJ Host", *updated.HostName)                       // was nil, filled
	suite.Equal("A great show", *updated.Description)               // was nil, filled
	suite.Equal("https://example.com/archive", *updated.ArchiveURL) // was nil, filled
}

func (suite *RadioImportIntegrationTestSuite) TestUpsertRadioShow_SlugFallback() {
	station := suite.createStation("KEXP")

	// Simulate a seeded show with the wrong external_id (the original bug).
	// The seed had external_id='1' for "The Morning Show", but the real
	// KEXP API program ID is '16'.
	wrongExtID := "1"
	show := &catalogm.RadioShow{
		StationID:  station.ID,
		Name:       "The Morning Show",
		Slug:       "the-morning-show",
		ExternalID: &wrongExtID,
	}
	suite.Require().NoError(suite.db.Create(show).Error)

	// Now upsert with the correct external_id from the API.
	// The slug "the-morning-show" should match, and external_id should be updated.
	importShow := RadioShowImport{
		ExternalID: "16",
		Name:       "The Morning Show",
		HostName:   stringPtr("John Richards"),
	}

	showID, _, err := suite.radioService.upsertRadioShow(station.ID, importShow)
	suite.Require().NoError(err)
	suite.Equal(show.ID, showID, "should match the existing show by slug, not create a new one")

	// Verify external_id was updated to the correct value
	var updated catalogm.RadioShow
	suite.db.First(&updated, showID)
	suite.Equal("16", *updated.ExternalID)
	suite.Equal("The Morning Show", updated.Name)
	suite.Equal("John Richards", *updated.HostName)

	// Verify no duplicate was created
	var count int64
	suite.db.Model(&catalogm.RadioShow{}).Where("station_id = ?", station.ID).Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *RadioImportIntegrationTestSuite) TestUpsertRadioShow_SlugFallbackDoesNotCrossStations() {
	station1 := suite.createStation("KEXP")
	station2 := suite.createStation("NTS")

	// Create show in station1 with slug "morning-show"
	extID := "1"
	show := &catalogm.RadioShow{
		StationID:  station1.ID,
		Name:       "Morning Show",
		Slug:       "morning-show",
		ExternalID: &extID,
	}
	suite.Require().NoError(suite.db.Create(show).Error)

	// Upsert a show with the same name but for station2 — should NOT match station1's show
	importShow := RadioShowImport{
		ExternalID: "99",
		Name:       "Morning Show",
	}

	showID, _, err := suite.radioService.upsertRadioShow(station2.ID, importShow)
	suite.Require().NoError(err)
	suite.NotEqual(show.ID, showID, "should not match show from a different station")
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
	suite.db.Model(&catalogm.RadioEpisode{}).Where("show_id = ?", show.ID).Count(&count)
	suite.Equal(int64(1), count)
}

// PSY-1119: a FetchPlaylist failure must NOT pass as a clean 0-play success.
// Before this fix, importEpisode returned an empty EpisodeImportResult on a
// fetch error, indistinguishable from a legitimately empty playlist — so the
// episode silently lost all its plays and the import reported success.
//
// This test drives importEpisode directly with a mock provider that errors on
// FetchPlaylist and asserts: (a) the episode row IS created (the failure is
// non-fatal to the batch), (b) FetchError is populated, and (c) PlaysImported
// is 0 — but flagged via FetchError, not a clean zero.
func (suite *RadioImportIntegrationTestSuite) TestImportEpisode_FetchPlaylistError_IsRecorded() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")

	mockProvider := &mockPlaylistProvider{
		fetchPlaylistFn: func(epExtID string) ([]RadioPlayImport, error) {
			return nil, fmt.Errorf("provider boom: upstream 500")
		},
	}

	ep := RadioEpisodeImport{ExternalID: "ep-fetch-fail", ShowExternalID: "42", AirDate: "2026-01-15"}

	epResult, err := suite.radioService.importEpisode(show.ID, ep, mockProvider)
	suite.Require().NoError(err, "fetch failure is non-fatal to the batch")
	suite.Require().NotNil(epResult)
	suite.Equal(0, epResult.PlaysImported)
	suite.NotEmpty(epResult.FetchError, "fetch failure must be recorded, not swallowed")
	suite.Contains(epResult.FetchError, "provider boom")

	// The episode row was still created (so a retry path can find it).
	var count int64
	suite.db.Model(&catalogm.RadioEpisode{}).
		Where("show_id = ? AND external_id = ?", show.ID, ep.ExternalID).Count(&count)
	suite.Equal(int64(1), count, "episode row should exist despite the fetch failure")
}

// PSY-1119: the complement of the above — a legitimately empty playlist
// (provider returns (nil, nil), as KEXP does for a 404 / no-start-time episode)
// must remain a CLEAN success: no FetchError, zero plays, no error surfaced.
func (suite *RadioImportIntegrationTestSuite) TestImportEpisode_EmptyPlaylist_IsCleanSuccess() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")

	mockProvider := &mockPlaylistProvider{
		fetchPlaylistFn: func(epExtID string) ([]RadioPlayImport, error) {
			return nil, nil // legitimately empty — NOT an error
		},
	}

	ep := RadioEpisodeImport{ExternalID: "ep-empty", ShowExternalID: "42", AirDate: "2026-01-15"}

	epResult, err := suite.radioService.importEpisode(show.ID, ep, mockProvider)
	suite.Require().NoError(err)
	suite.Require().NotNil(epResult)
	suite.Equal(0, epResult.PlaysImported)
	suite.Empty(epResult.FetchError, "an empty playlist is not a fetch error")
	suite.Empty(epResult.DropSummary)
}

// PSY-1119: the station orchestrator must aggregate per-episode fetch failures
// into RadioImportResult.EpisodeFetchErrors and Errors, so a job that loses
// plays reports a distinct, queryable state instead of a clean completion. One
// show, two episodes: the first fetches plays cleanly, the second errors.
func (suite *RadioImportIntegrationTestSuite) TestImport_AggregatesFetchErrors() {
	station := suite.createStation("KEXP")

	mockProvider := &mockPlaylistProvider{
		discoverShowsFn: func() ([]RadioShowImport, error) {
			return []RadioShowImport{{Name: "Morning Show", ExternalID: "show-1"}}, nil
		},
		fetchNewEpisodesFn: func(showExtID string, since, until time.Time) ([]RadioEpisodeImport, error) {
			return []RadioEpisodeImport{
				{ExternalID: "ep-ok", ShowExternalID: "show-1", AirDate: "2026-01-15"},
				{ExternalID: "ep-boom", ShowExternalID: "show-1", AirDate: "2026-01-16"},
			}, nil
		},
		fetchPlaylistFn: func(epExtID string) ([]RadioPlayImport, error) {
			if epExtID == "ep-boom" {
				return nil, fmt.Errorf("upstream 503 for %s", epExtID)
			}
			return []RadioPlayImport{{Position: 0, ArtistName: "Radiohead"}}, nil
		},
	}

	result := suite.runImportWithProvider(station.ID, 30, mockProvider)

	suite.Equal(1, result.ShowsDiscovered)
	suite.Equal(2, result.EpisodesImported, "both episode rows were created")
	suite.Equal(1, result.PlaysImported, "only the healthy episode contributed plays")
	suite.Equal(1, result.EpisodeFetchErrors, "the errored episode must be counted")
	suite.Require().NotEmpty(result.Errors)
	// The fetch-failure line must be present and identify the episode.
	foundFetchErr := false
	for _, e := range result.Errors {
		if strings.Contains(e, "fetch failed for episode ep-boom") {
			foundFetchErr = true
		}
	}
	suite.True(foundFetchErr, "errors should record the failed-fetch episode: %v", result.Errors)
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

	count, dropSummary, err := suite.radioService.importPlays(ep.ID, plays)

	suite.Require().NoError(err)
	suite.Equal(3, count)
	suite.Empty(dropSummary, "clean batch should produce no drop summary")

	// Verify plays in DB
	var dbPlays []catalogm.RadioPlay
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

	count, dropSummary, err := suite.radioService.importPlays(ep.ID, []RadioPlayImport{})

	suite.Require().NoError(err)
	suite.Equal(0, count)
	suite.Empty(dropSummary)
}

// PSY-885: validate-at-boundary tests for importPlays. Cover the four cases
// enumerated in the ticket:
//   - clean batch       → full count, empty summary
//   - over-length title → truncated to 500 runes, summary records "truncated"
//   - NULL artist_name  → row dropped, summary records "missing artist_name"
//   - mixed batch       → only valid rows committed, summary reflects both
//
// Returned count is rows COMMITTED — drops are excluded.

func (suite *RadioImportIntegrationTestSuite) TestImportPlays_TruncatesOverLengthArtistName() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	// 600-char artist name (overflows VARCHAR(500))
	overLength := strings.Repeat("a", 600)
	plays := []RadioPlayImport{
		{Position: 0, ArtistName: overLength},
	}

	count, dropSummary, err := suite.radioService.importPlays(ep.ID, plays)
	suite.Require().NoError(err)
	suite.Equal(1, count, "truncated row should still be committed")
	// Summary counts truncated rows in N (per PSY-885 format spec) — "dropped"
	// is used loosely to mean "required boundary intervention", with the
	// per-class breakdown distinguishing salvage from data loss.
	suite.Contains(dropSummary, "dropped 1 plays")
	suite.Contains(dropSummary, "1 over-length titles truncated")

	var dbPlays []catalogm.RadioPlay
	suite.db.Where("episode_id = ?", ep.ID).Find(&dbPlays)
	suite.Len(dbPlays, 1)
	suite.Equal(strings.Repeat("a", 500), dbPlays[0].ArtistName, "artist_name should be trimmed to 500 runes")
}

func (suite *RadioImportIntegrationTestSuite) TestImportPlays_TruncatesOverLengthOptionalFields() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	overTitle := strings.Repeat("b", 600)
	overAlbum := strings.Repeat("c", 700)
	overLabel := strings.Repeat("d", 501)
	plays := []RadioPlayImport{
		{Position: 0, ArtistName: "Boundary Band", TrackTitle: &overTitle, AlbumTitle: &overAlbum, LabelName: &overLabel},
	}

	count, dropSummary, err := suite.radioService.importPlays(ep.ID, plays)
	suite.Require().NoError(err)
	suite.Equal(1, count)
	suite.Contains(dropSummary, "1 over-length titles truncated", "a single row with multiple over-length fields counts once")

	var dbPlays []catalogm.RadioPlay
	suite.db.Where("episode_id = ?", ep.ID).Find(&dbPlays)
	suite.Len(dbPlays, 1)
	suite.Equal("Boundary Band", dbPlays[0].ArtistName)
	suite.Require().NotNil(dbPlays[0].TrackTitle)
	suite.Len([]rune(*dbPlays[0].TrackTitle), 500)
	suite.Require().NotNil(dbPlays[0].AlbumTitle)
	suite.Len([]rune(*dbPlays[0].AlbumTitle), 500)
	suite.Require().NotNil(dbPlays[0].LabelName)
	suite.Len([]rune(*dbPlays[0].LabelName), 500)
}

func (suite *RadioImportIntegrationTestSuite) TestImportPlays_TruncatesMultiByteRunesAtBoundary() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	// Each "é" is 2 UTF-8 bytes but 1 rune. 600 runes overflows.
	overLength := strings.Repeat("é", 600)
	plays := []RadioPlayImport{
		{Position: 0, ArtistName: overLength},
	}

	count, _, err := suite.radioService.importPlays(ep.ID, plays)
	suite.Require().NoError(err, "truncation must respect rune boundaries, not split a multi-byte char")
	suite.Equal(1, count)

	var dbPlays []catalogm.RadioPlay
	suite.db.Where("episode_id = ?", ep.ID).Find(&dbPlays)
	suite.Len(dbPlays, 1)
	// Postgres counts characters (runes), not bytes — should fit 500 "é"s exactly.
	suite.Equal(500, len([]rune(dbPlays[0].ArtistName)), "trimmed to 500 runes")
	suite.True(utf8.ValidString(dbPlays[0].ArtistName), "trimmed string must remain valid UTF-8")
}

func (suite *RadioImportIntegrationTestSuite) TestImportPlays_DropsMissingArtistName() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	plays := []RadioPlayImport{
		{Position: 0, ArtistName: ""},
		{Position: 1, ArtistName: "   "}, // whitespace-only also dropped
	}

	count, dropSummary, err := suite.radioService.importPlays(ep.ID, plays)
	suite.Require().NoError(err)
	suite.Equal(0, count, "rows with NULL/blank artist_name must be dropped")
	suite.Contains(dropSummary, "dropped 2 plays")
	suite.Contains(dropSummary, "2 missing artist_name")

	var playCount int64
	suite.db.Model(&catalogm.RadioPlay{}).Where("episode_id = ?", ep.ID).Count(&playCount)
	suite.Equal(int64(0), playCount)
}

func (suite *RadioImportIntegrationTestSuite) TestImportPlays_MixedBatch() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	overLength := strings.Repeat("x", 600)
	plays := []RadioPlayImport{
		{Position: 0, ArtistName: "Radiohead"},          // clean
		{Position: 1, ArtistName: ""},                   // dropped: blank artist
		{Position: 2, ArtistName: overLength},           // truncated
		{Position: 3, ArtistName: "Deerhunter"},         // clean
		{Position: 4, ArtistName: "Sonic Youth"},        // clean
		{Position: 5, ArtistName: "  \t  "},             // dropped: whitespace-only
	}

	count, dropSummary, err := suite.radioService.importPlays(ep.ID, plays)
	suite.Require().NoError(err)
	// 4 rows commit: Radiohead, Deerhunter, Sonic Youth, truncated overLength row.
	// 2 rows drop: the two blank artist_name rows.
	suite.Equal(4, count, "return value must reflect rows COMMITTED, not rows received")

	// Summary covers BOTH classes in one line, no per-play entries. The
	// PSY-885 format counts truncated + missing as the leading N: 1 + 2 = 3.
	suite.Contains(dropSummary, "dropped 3 plays")
	suite.Contains(dropSummary, "1 over-length titles truncated")
	suite.Contains(dropSummary, "2 missing artist_name")
	suite.Equal(1, strings.Count(dropSummary, "\n")+1, "summary must be a single line")

	var dbPlays []catalogm.RadioPlay
	suite.db.Where("episode_id = ?", ep.ID).Order("position ASC").Find(&dbPlays)
	suite.Len(dbPlays, 4)
	suite.Equal("Radiohead", dbPlays[0].ArtistName)
	suite.Equal(strings.Repeat("x", 500), dbPlays[1].ArtistName, "truncated row preserved at its position")
	suite.Equal("Deerhunter", dbPlays[2].ArtistName)
	suite.Equal("Sonic Youth", dbPlays[3].ArtistName)
}

// PSY-888: Re-importing the same playlist must succeed without error and
// without inserting duplicate rows. Before the ON CONFLICT DO NOTHING +
// idx_radio_plays_unique migration, the second call rolled back the whole
// 100-row batch and returned an error.
func (suite *RadioImportIntegrationTestSuite) TestImportPlays_DedupOnReimport() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	track1 := "Karma Police"
	track2 := "Cover Me"
	ts1 := time.Date(2026, 1, 15, 9, 5, 0, 0, time.UTC)
	ts2 := time.Date(2026, 1, 15, 9, 10, 0, 0, time.UTC)

	plays := []RadioPlayImport{
		{Position: 0, ArtistName: "Radiohead", TrackTitle: &track1, AirTimestamp: &ts1},
		{Position: 1, ArtistName: "Deerhunter", TrackTitle: &track2, AirTimestamp: &ts2},
		// NULL track + NULL air_timestamp — covers the NTS-style case where
		// NULLS NOT DISTINCT must engage for dedup to work.
		{Position: 2, ArtistName: "Sonic Youth"},
	}

	// First import — all three rows inserted.
	count, _, err := suite.radioService.importPlays(ep.ID, plays)
	suite.Require().NoError(err)
	suite.Equal(3, count)

	var dbCount int64
	suite.db.Model(&catalogm.RadioPlay{}).Where("episode_id = ?", ep.ID).Count(&dbCount)
	suite.Equal(int64(3), dbCount, "first import should insert all 3 rows")

	// Second import (re-fetch) — same playlist, no new rows, no error.
	count2, _, err := suite.radioService.importPlays(ep.ID, plays)
	suite.Require().NoError(err, "re-importing duplicates must not error")
	suite.Equal(3, count2, "importPlays returns attempted count for play_count stability")

	suite.db.Model(&catalogm.RadioPlay{}).Where("episode_id = ?", ep.ID).Count(&dbCount)
	suite.Equal(int64(3), dbCount, "re-import must not insert duplicate rows")
}

// PSY-888: A mixed batch (some duplicates of prior rows + some genuinely
// new rows) should insert ONLY the new rows and not fail. This is the
// real-world re-fetch-with-new-songs case.
func (suite *RadioImportIntegrationTestSuite) TestImportPlays_PartialOverlapInsertsNewOnly() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	track1 := "Karma Police"
	track2 := "Cover Me"
	track3 := "Mountain"
	ts1 := time.Date(2026, 1, 15, 9, 5, 0, 0, time.UTC)
	ts2 := time.Date(2026, 1, 15, 9, 10, 0, 0, time.UTC)
	ts3 := time.Date(2026, 1, 15, 9, 15, 0, 0, time.UTC)

	firstBatch := []RadioPlayImport{
		{Position: 0, ArtistName: "Radiohead", TrackTitle: &track1, AirTimestamp: &ts1},
		{Position: 1, ArtistName: "Deerhunter", TrackTitle: &track2, AirTimestamp: &ts2},
	}
	_, _, err := suite.radioService.importPlays(ep.ID, firstBatch)
	suite.Require().NoError(err)

	// Second fetch — first two are dupes, third is new.
	secondBatch := []RadioPlayImport{
		{Position: 0, ArtistName: "Radiohead", TrackTitle: &track1, AirTimestamp: &ts1},
		{Position: 1, ArtistName: "Deerhunter", TrackTitle: &track2, AirTimestamp: &ts2},
		{Position: 2, ArtistName: "Cocteau Twins", TrackTitle: &track3, AirTimestamp: &ts3},
	}
	_, _, err = suite.radioService.importPlays(ep.ID, secondBatch)
	suite.Require().NoError(err, "partial overlap must not roll back the batch")

	var dbCount int64
	suite.db.Model(&catalogm.RadioPlay{}).Where("episode_id = ?", ep.ID).Count(&dbCount)
	suite.Equal(int64(3), dbCount, "must end with 3 rows (2 original + 1 new)")
}

// PSY-888: ON CONFLICT DO NOTHING only masks UNIQUE violations. Other
// constraint failures (e.g. NOT NULL or FK) must still surface as errors
// to the caller — they indicate a genuine data bug, not a re-import
// collision. AC explicitly calls this out.
//
// Real-world plays always carry a non-empty ArtistName (providers skip
// blank artists), so we use a FK violation instead — a play with a
// non-existent episode_id triggers the FK to radio_episodes. FK is a
// different constraint kind than UNIQUE, so ON CONFLICT DO NOTHING does
// NOT swallow it.
func (suite *RadioImportIntegrationTestSuite) TestImportPlays_NonUniqueConstraintViolationErrors() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	plays := []RadioPlayImport{
		{Position: 0, ArtistName: "Radiohead"},
	}

	// Use a non-existent episode ID so the FK to radio_episodes fires.
	_, _, err := suite.radioService.importPlays(ep.ID+99999, plays)
	suite.Require().Error(err, "non-UNIQUE constraint failures must still surface as errors")
}

func (suite *RadioImportIntegrationTestSuite) TestMatchPlays_LabelCaseInsensitive() {
	station := suite.createStation("KEXP")
	show := suite.createShow(station.ID, "Morning Show")
	ep := suite.createEpisode(show.ID, "2026-01-15")

	labelName := "sub pop" // lowercase
	play := &catalogm.RadioPlay{
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

	var updated catalogm.RadioPlay
	suite.db.First(&updated, play.ID)
	suite.NotNil(updated.LabelID) // Case-insensitive match
}

func (suite *RadioImportIntegrationTestSuite) TestGetProvider_Unsupported() {
	_, err := suite.radioService.getProvider("unsupported_source")
	suite.Error(err)
	suite.Contains(err.Error(), "unsupported playlist source")
}

func (suite *RadioImportIntegrationTestSuite) TestGetProvider_Manual() {
	// PSY-927: 'manual' is a valid source with no automated provider. It must
	// return its own error (caller skips the station) WITHOUT folding into the
	// loud "unsupported playlist source" default branch reserved for typos.
	_, err := suite.radioService.getProvider(catalogm.PlaylistSourceManual)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "manual")
	suite.Contains(err.Error(), "no automated provider")
	suite.NotContains(err.Error(), "unsupported playlist source")
}

func (suite *RadioImportIntegrationTestSuite) TestGetProvider_KEXP() {
	provider, err := suite.radioService.getProvider(catalogm.PlaylistSourceKEXP)
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
		BroadcastType: catalogm.BroadcastTypeBoth,
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

func (suite *RadioImportIntegrationTestSuite) createEpisode(showID uint, airDate string) *catalogm.RadioEpisode {
	ep := &catalogm.RadioEpisode{
		ShowID:  showID,
		AirDate: airDate,
	}
	err := suite.db.Create(ep).Error
	suite.Require().NoError(err)
	return ep
}

func (suite *RadioImportIntegrationTestSuite) createPlay(episodeID uint, position int, artistName string) *catalogm.RadioPlay {
	play := &catalogm.RadioPlay{
		EpisodeID:  episodeID,
		Position:   position,
		ArtistName: artistName,
	}
	err := suite.db.Create(play).Error
	suite.Require().NoError(err)
	return play
}

func (suite *RadioImportIntegrationTestSuite) createArtist(name string) *catalogm.Artist {
	slug := utils.GenerateArtistSlug(name)
	artist := &catalogm.Artist{Name: name, Slug: &slug}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *RadioImportIntegrationTestSuite) createRelease(title string) *catalogm.Release {
	release := &catalogm.Release{Title: title}
	err := suite.db.Create(release).Error
	suite.Require().NoError(err)
	return release
}

func (suite *RadioImportIntegrationTestSuite) createLabel(name string) *catalogm.Label {
	label := &catalogm.Label{Name: name, Status: catalogm.LabelStatusActive}
	err := suite.db.Create(label).Error
	suite.Require().NoError(err)
	return label
}

// runImportWithProvider runs the import pipeline with a specific provider (bypassing getProvider).
func (suite *RadioImportIntegrationTestSuite) runImportWithProvider(stationID uint, backfillDays int, provider RadioPlaylistProvider) *contracts.RadioImportResult {
	var station catalogm.RadioStation
	suite.Require().NoError(suite.db.First(&station, stationID).Error)

	result := &contracts.RadioImportResult{}

	importedShows, err := provider.DiscoverShows()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("discover shows: %v", err))
		return result
	}

	showMap := make(map[string]uint)
	for _, importShow := range importedShows {
		showID, _, err := suite.radioService.upsertRadioShow(stationID, importShow)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("upsert show: %v", err))
			continue
		}
		showMap[importShow.ExternalID] = showID
		result.ShowsDiscovered++
	}

	since := time.Now().AddDate(0, 0, -backfillDays)
	for extID, showID := range showMap {
		episodes, err := provider.FetchNewEpisodes(extID, since, time.Time{})
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
			// Use the production aggregation so the test helper reflects the real
			// orchestrators' handling of fetch / match-persist errors (PSY-1119).
			accumulateEpisodeResult(result, ep.ExternalID, epResult)
		}
	}

	return result
}

// =============================================================================
// Mock provider
// =============================================================================

type mockPlaylistProvider struct {
	discoverShowsFn    func() ([]RadioShowImport, error)
	fetchNewEpisodesFn func(showExternalID string, since time.Time, until time.Time) ([]RadioEpisodeImport, error)
	fetchPlaylistFn    func(episodeExternalID string) ([]RadioPlayImport, error)
}

func (m *mockPlaylistProvider) DiscoverShows() ([]RadioShowImport, error) {
	if m.discoverShowsFn != nil {
		return m.discoverShowsFn()
	}
	return nil, nil
}

func (m *mockPlaylistProvider) FetchNewEpisodes(showExternalID string, since time.Time, until time.Time) ([]RadioEpisodeImport, error) {
	if m.fetchNewEpisodesFn != nil {
		return m.fetchNewEpisodesFn(showExternalID, since, until)
	}
	return nil, nil
}

func (m *mockPlaylistProvider) FetchPlaylist(episodeExternalID string) ([]RadioPlayImport, error) {
	if m.fetchPlaylistFn != nil {
		return m.fetchPlaylistFn(episodeExternalID)
	}
	return nil, nil
}
