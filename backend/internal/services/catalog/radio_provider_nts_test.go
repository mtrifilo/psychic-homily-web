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

	"psychic-homily-backend/internal/models"
)

// =============================================================================
// Test fixtures — realistic NTS API JSON responses
// =============================================================================

const ntsShowsPage1JSON = `{
  "results": [
    {
      "name": "Huerco S.",
      "alias": "huerco-s",
      "description": "Kansas-born ambient producer Brian Leeds presents a monthly show exploring the fringes of experimental music.",
      "description_short": "Ambient explorations with Brian Leeds",
      "genre_tags": ["ambient", "experimental", "drone"],
      "mood_tags": ["deep", "meditative", "nocturnal"],
      "location": "Kansas, USA",
      "image_url": "https://media.nts.live/uploads/huerco-s.jpg",
      "hosts": [{"name": "Brian Leeds"}]
    },
    {
      "name": "Scratcha DVA",
      "alias": "scratcha-dva",
      "description": "London DJ and producer Scratcha DVA mixing bass, grime, and club music from the diaspora.",
      "genre_tags": ["grime", "bass", "club"],
      "mood_tags": ["energetic", "dark"],
      "location": "London, UK",
      "image_url": "https://media.nts.live/uploads/scratcha-dva.jpg",
      "hosts": [{"name": "Scratcha DVA"}]
    },
    {
      "name": "Morning Becomes Eclectic",
      "alias": "morning-becomes-eclectic",
      "description": "An eclectic morning show with no singular host, featuring guest DJs and live sessions.",
      "genre_tags": ["eclectic", "indie", "world"],
      "mood_tags": ["uplifting", "warm"],
      "location": "London, UK",
      "image_url": "",
      "hosts": []
    }
  ]
}`

const ntsShowsPage2JSON = `{
  "results": [
    {
      "name": "Donato Dozzy",
      "alias": "donato-dozzy",
      "description": "Legendary Italian techno producer showcasing hypnotic rhythms and deep electronics.",
      "genre_tags": ["techno", "minimal", "hypnotic"],
      "mood_tags": ["hypnotic", "deep"],
      "location": "Rome, Italy",
      "image_url": "https://media.nts.live/uploads/donato-dozzy.jpg",
      "hosts": [{"name": "Donato Dozzy"}]
    }
  ]
}`

const ntsEpisodesJSON = `{
  "results": [
    {
      "name": "Huerco S. - March 2026",
      "episode_alias": "march-2026",
      "broadcast": "2026-03-15T20:00:00Z",
      "mixcloud": "https://www.mixcloud.com/NTSRadio/huerco-s-15th-march-2026/",
      "genre_tags": ["ambient", "experimental"],
      "mood_tags": ["meditative", "nocturnal"],
      "duration": 120
    },
    {
      "name": "Huerco S. - February 2026",
      "episode_alias": "february-2026",
      "broadcast": "2026-02-15T20:00:00Z",
      "mixcloud": "https://www.mixcloud.com/NTSRadio/huerco-s-15th-february-2026/",
      "genre_tags": ["ambient", "drone"],
      "mood_tags": ["deep"],
      "duration": 60
    },
    {
      "name": "Huerco S. - January 2026",
      "episode_alias": "january-2026",
      "broadcast": "2026-01-15T20:00:00Z",
      "mixcloud": "https://www.mixcloud.com/NTSRadio/huerco-s-15th-january-2026/",
      "genre_tags": ["ambient"],
      "mood_tags": [],
      "duration": 120
    }
  ]
}`

const ntsEpisodeOlderJSON = `{
  "results": [
    {
      "name": "Huerco S. - December 2025",
      "episode_alias": "december-2025",
      "broadcast": "2025-12-15T20:00:00Z",
      "mixcloud": "https://www.mixcloud.com/NTSRadio/huerco-s-15th-december-2025/",
      "genre_tags": ["ambient"],
      "mood_tags": [],
      "duration": 120
    }
  ]
}`

// ntsTracklistJSON is a realistic response from
// GET /v2/shows/{alias}/episodes/{ep_alias}/tracklist. The real NTS API
// wraps tracks in a `results` array under a metadata/resultset envelope.
const ntsTracklistJSON = `{
  "metadata": {
    "resultset": {"count": 5}
  },
  "results": [
    {"artist": "Grouper", "title": "Holding", "uid": "uid-1", "offset": 0, "duration": 240},
    {"artist": "Stars of the Lid", "title": "Requiem for Dying Mothers, Part 2", "uid": "uid-2", "offset": 241, "duration": 600},
    {"artist": "Midori Takada", "title": "Mr. Henri Rousseau's Dream", "uid": "uid-3", "offset": 842, "duration": 420},
    {"artist": "Pauline Anna Strom", "title": "Trans-Millenia Consort", "uid": "uid-4", "offset": 1263, "duration": 360},
    {"artist": "Hiroshi Yoshimura", "title": "Creek", "uid": "uid-5", "offset": 1624, "duration": 300}
  ]
}`

// ntsEmptyTracklistJSON is what the tracklist endpoint returns for episodes
// that have no tracklist entered (common for DJ mixes). It's a 200 response
// with an empty results array.
const ntsEmptyTracklistJSON = `{
  "metadata": {
    "resultset": {"count": 0}
  },
  "results": []
}`

const ntsEmptyShowsJSON = `{"results": []}`
const ntsEmptyEpisodesJSON = `{"results": []}`

// =============================================================================
// Show Discovery Tests
// =============================================================================

func TestNTS_DiscoverShows_ParsesAllFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, ntsShowsPage1JSON)
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	shows, err := provider.DiscoverShows()
	require.NoError(t, err)
	assert.Len(t, shows, 3)

	// Check Huerco S. — full metadata
	h := shows[0]
	assert.Equal(t, "huerco-s", h.ExternalID)
	assert.Equal(t, "Huerco S.", h.Name)
	require.NotNil(t, h.Description)
	assert.Contains(t, *h.Description, "ambient producer")
	require.NotNil(t, h.HostName)
	assert.Equal(t, "Brian Leeds", *h.HostName)
	require.NotNil(t, h.ImageURL)
	assert.Equal(t, "https://media.nts.live/uploads/huerco-s.jpg", *h.ImageURL)
	require.NotNil(t, h.ArchiveURL)
	assert.Equal(t, "https://www.nts.live/shows/huerco-s", *h.ArchiveURL)

	// Check Scratcha DVA — different host
	s := shows[1]
	assert.Equal(t, "scratcha-dva", s.ExternalID)
	assert.Equal(t, "Scratcha DVA", s.Name)
	require.NotNil(t, s.HostName)
	assert.Equal(t, "Scratcha DVA", *s.HostName)

	// Check Morning Becomes Eclectic — no host, no image
	m := shows[2]
	assert.Equal(t, "morning-becomes-eclectic", m.ExternalID)
	assert.Equal(t, "Morning Becomes Eclectic", m.Name)
	assert.Nil(t, m.HostName, "show with no hosts should have nil HostName")
	assert.Nil(t, m.ImageURL, "show with empty image_url should have nil ImageURL")
}

func TestNTS_DiscoverShows_Pagination(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		offset := r.URL.Query().Get("offset")
		if offset == "" || offset == "0" {
			// First page: return exactly ntsPageLimit results to trigger next page fetch
			results := make([]ntsShow, ntsPageLimit)
			for i := 0; i < ntsPageLimit; i++ {
				results[i] = ntsShow{
					Name:  fmt.Sprintf("Show %d", i),
					Alias: fmt.Sprintf("show-%d", i),
				}
			}
			data, _ := json.Marshal(ntsShowsResponse{Results: results})
			w.Write(data)
		} else {
			// Second page: return fewer results (signals end of pagination)
			fmt.Fprint(w, ntsShowsPage2JSON)
		}
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	shows, err := provider.DiscoverShows()
	require.NoError(t, err)
	assert.Len(t, shows, ntsPageLimit+1, "should combine results from both pages")
	assert.Equal(t, 2, requestCount, "should make 2 requests for pagination")
}

func TestNTS_DiscoverShows_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, ntsEmptyShowsJSON)
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	shows, err := provider.DiscoverShows()
	require.NoError(t, err)
	assert.Empty(t, shows)
}

// =============================================================================
// Episode Discovery Tests
// =============================================================================

func TestNTS_FetchNewEpisodes_AllFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, ntsEpisodesJSON)
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	episodes, err := provider.FetchNewEpisodes("huerco-s", since, time.Time{})
	require.NoError(t, err)
	assert.Len(t, episodes, 3)

	// Check first episode — full metadata
	ep := episodes[0]
	assert.Equal(t, "huerco-s/march-2026", ep.ExternalID)
	assert.Equal(t, "huerco-s", ep.ShowExternalID)
	assert.Equal(t, "2026-03-15", ep.AirDate)
	require.NotNil(t, ep.AirTime)
	assert.Equal(t, "20:00:00", *ep.AirTime)
	require.NotNil(t, ep.Title)
	assert.Equal(t, "Huerco S. - March 2026", *ep.Title)
	require.NotNil(t, ep.ArchiveURL)
	assert.Contains(t, *ep.ArchiveURL, "mixcloud.com")
	require.NotNil(t, ep.DurationMinutes)
	assert.Equal(t, 120, *ep.DurationMinutes)
}

func TestNTS_FetchNewEpisodes_DateFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, ntsEpisodesJSON)
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	// Only get episodes since Feb 1, 2026
	since := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	episodes, err := provider.FetchNewEpisodes("huerco-s", since, time.Time{})
	require.NoError(t, err)
	assert.Len(t, episodes, 2, "should only return episodes after Feb 1, 2026")

	// Both should be Feb or March 2026
	assert.Equal(t, "2026-03-15", episodes[0].AirDate)
	assert.Equal(t, "2026-02-15", episodes[1].AirDate)
}

func TestNTS_FetchNewEpisodes_StopsAtOldEpisodes(t *testing.T) {
	page := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		w.Header().Set("Content-Type", "application/json")
		if page == 1 {
			// Return full page to trigger pagination
			results := make([]ntsEpisode, ntsPageLimit)
			for i := 0; i < ntsPageLimit; i++ {
				results[i] = ntsEpisode{
					Name:         fmt.Sprintf("Episode %d", i),
					EpisodeAlias: fmt.Sprintf("ep-%d", i),
					Broadcast:    "2026-03-15T20:00:00Z",
				}
			}
			data, _ := json.Marshal(ntsEpisodesResponse{Results: results})
			w.Write(data)
		} else {
			// Second page has old episodes
			fmt.Fprint(w, ntsEpisodeOlderJSON)
		}
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	episodes, err := provider.FetchNewEpisodes("huerco-s", since, time.Time{})
	require.NoError(t, err)
	// Should have ntsPageLimit from page 1, and 0 from page 2 (old episode filtered)
	assert.Equal(t, ntsPageLimit, len(episodes))
}

func TestNTS_FetchNewEpisodes_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, ntsEmptyEpisodesJSON)
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	episodes, err := provider.FetchNewEpisodes("nonexistent", time.Now(), time.Time{})
	require.NoError(t, err)
	assert.Empty(t, episodes)
}

// =============================================================================
// Playlist (Tracklist) Tests
// =============================================================================

func TestNTS_FetchPlaylist_WithTracklist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// FetchPlaylist must call the /tracklist sub-endpoint, not the
		// episode detail endpoint — the latter does not include tracklist
		// data and every import was coming back empty because of it.
		assert.Equal(t, "/v2/shows/huerco-s/episodes/march-2026/tracklist", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, ntsTracklistJSON)
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	plays, err := provider.FetchPlaylist("huerco-s/march-2026")
	require.NoError(t, err)
	assert.Len(t, plays, 5)

	// Check first track
	p0 := plays[0]
	assert.Equal(t, 0, p0.Position)
	assert.Equal(t, "Grouper", p0.ArtistName)
	require.NotNil(t, p0.TrackTitle)
	assert.Equal(t, "Holding", *p0.TrackTitle)

	// NTS tracklist endpoint exposes only artist + title -- no album, label,
	// MusicBrainz IDs, release year, or wall-clock air timestamp. `offset`
	// and `duration` are seconds within the episode audio, not times of day.
	assert.Nil(t, p0.AlbumTitle)
	assert.Nil(t, p0.LabelName)
	assert.Nil(t, p0.ReleaseYear)
	assert.Nil(t, p0.MusicBrainzArtistID)
	assert.Nil(t, p0.MusicBrainzRecordingID)
	assert.Nil(t, p0.MusicBrainzReleaseID)
	assert.Nil(t, p0.AirTimestamp)

	// Check middle track — verify position numbering
	p2 := plays[2]
	assert.Equal(t, 2, p2.Position)
	assert.Equal(t, "Midori Takada", p2.ArtistName)
	require.NotNil(t, p2.TrackTitle)
	assert.Equal(t, "Mr. Henri Rousseau's Dream", *p2.TrackTitle)

	// Check last track
	p4 := plays[4]
	assert.Equal(t, 4, p4.Position)
	assert.Equal(t, "Hiroshi Yoshimura", p4.ArtistName)
	require.NotNil(t, p4.TrackTitle)
	assert.Equal(t, "Creek", *p4.TrackTitle)
}

func TestNTS_FetchPlaylist_EmptyTracklist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v2/shows/scratcha-dva/episodes/march-2026-mix/tracklist", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, ntsEmptyTracklistJSON)
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	plays, err := provider.FetchPlaylist("scratcha-dva/march-2026-mix")
	require.NoError(t, err)
	assert.NotNil(t, plays, "empty tracklist should return non-nil empty slice")
	assert.Len(t, plays, 0, "DJ mix episodes should return 0 plays")
}

// NTS returns 404 for episodes that have no tracklist at all (not just an
// empty array). This is the normal case for DJ mixes and should not be
// treated as an error.
func TestNTS_FetchPlaylist_TracklistNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"detail": "Not found."}`)
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	plays, err := provider.FetchPlaylist("ambient-show/ambient-session")
	require.NoError(t, err, "404 on tracklist endpoint should not be an error")
	assert.NotNil(t, plays, "404 should return non-nil empty slice")
	assert.Len(t, plays, 0)
}

func TestNTS_FetchPlaylist_InvalidExternalID(t *testing.T) {
	provider := NewNTSProviderWithClient(&http.Client{}, "http://localhost")
	defer provider.Close()

	cases := []string{
		"invalid-no-slash",
		"",
		"show-only/",
		"/ep-only",
	}
	for _, id := range cases {
		_, err := provider.FetchPlaylist(id)
		assert.Error(t, err, "expected error for %q", id)
		if err != nil {
			assert.Contains(t, err.Error(), "invalid episode external ID format")
		}
	}
}

func TestNTS_FetchPlaylist_SkipsEmptyArtist(t *testing.T) {
	tracklistJSON := `{
		"metadata": {"resultset": {"count": 3}},
		"results": [
			{"artist": "Grouper", "title": "Holding"},
			{"artist": "", "title": "Unknown Track"},
			{"artist": "Stars of the Lid", "title": "Requiem"}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, tracklistJSON)
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	plays, err := provider.FetchPlaylist("test-show/test")
	require.NoError(t, err)
	assert.Len(t, plays, 2, "should skip track with empty artist")

	// Positions should be re-numbered sequentially
	assert.Equal(t, 0, plays[0].Position)
	assert.Equal(t, "Grouper", plays[0].ArtistName)
	assert.Equal(t, 1, plays[1].Position)
	assert.Equal(t, "Stars of the Lid", plays[1].ArtistName)
}

// =============================================================================
// Genre and Mood Tag Tests
// =============================================================================

func TestNTS_GenreAndMoodTags_Extraction(t *testing.T) {
	// Verify the NTS API response types properly capture genre and mood tags
	var showResp ntsShowsResponse
	err := json.Unmarshal([]byte(ntsShowsPage1JSON), &showResp)
	require.NoError(t, err)

	huerco := showResp.Results[0]
	assert.Equal(t, []string{"ambient", "experimental", "drone"}, huerco.GenreTags)
	assert.Equal(t, []string{"deep", "meditative", "nocturnal"}, huerco.MoodTags)
	assert.Equal(t, "Kansas, USA", huerco.Location)

	scratcha := showResp.Results[1]
	assert.Equal(t, []string{"grime", "bass", "club"}, scratcha.GenreTags)
	assert.Equal(t, []string{"energetic", "dark"}, scratcha.MoodTags)

	// Episode tags
	var epResp ntsEpisodesResponse
	err = json.Unmarshal([]byte(ntsEpisodesJSON), &epResp)
	require.NoError(t, err)

	ep := epResp.Results[0]
	assert.Equal(t, []string{"ambient", "experimental"}, ep.GenreTags)
	assert.Equal(t, []string{"meditative", "nocturnal"}, ep.MoodTags)
}

func TestNTS_EncodeTagsJSON(t *testing.T) {
	// Non-empty tags
	result := encodeTagsJSON([]string{"ambient", "experimental"})
	require.NotNil(t, result)

	var decoded []string
	err := json.Unmarshal(*result, &decoded)
	require.NoError(t, err)
	assert.Equal(t, []string{"ambient", "experimental"}, decoded)

	// Empty tags
	assert.Nil(t, encodeTagsJSON([]string{}))

	// Nil tags
	assert.Nil(t, encodeTagsJSON(nil))
}

// =============================================================================
// Mixcloud Archive URL Tests
// =============================================================================

func TestNTS_MixcloudArchiveURL_Preserved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, ntsEpisodesJSON)
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	episodes, err := provider.FetchNewEpisodes("huerco-s", since, time.Time{})
	require.NoError(t, err)

	for _, ep := range episodes {
		require.NotNil(t, ep.ArchiveURL, "every NTS episode should have a Mixcloud archive URL")
		assert.Contains(t, *ep.ArchiveURL, "mixcloud.com")
	}
}

// =============================================================================
// Rate Limiting Tests
// =============================================================================

func TestNTS_RateLimiting(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, ntsEmptyShowsJSON)
	}))
	defer server.Close()

	// Create provider with known rate limit
	provider := &NTSProvider{
		httpClient:  server.Client(),
		baseURL:     server.URL,
		rateLimiter: time.NewTicker(50 * time.Millisecond),
	}
	defer provider.Close()

	// Make 3 requests and measure time
	start := time.Now()

	_, _ = provider.DiscoverShows()
	_, _ = provider.FetchNewEpisodes("test", time.Now(), time.Time{})
	_, _ = provider.FetchPlaylist("test/ep1")

	elapsed := time.Since(start)

	// With 50ms rate limit and 3 requests, should take at least 100ms
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(80),
		"3 requests with 50ms rate limit should take at least ~100ms, took %v", elapsed)
}

// =============================================================================
// Error Handling Tests
// =============================================================================

func TestNTS_HTTPError_DiscoverShows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Internal Server Error")
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	_, err := provider.DiscoverShows()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestNTS_HTTPError_FetchNewEpisodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Not Found")
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	_, err := provider.FetchNewEpisodes("nonexistent", time.Now(), time.Time{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestNTS_HTTPError_FetchPlaylist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "Service Unavailable")
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	_, err := provider.FetchPlaylist("test/episode")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

func TestNTS_MalformedJSON_DiscoverShows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "this is not json")
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	_, err := provider.DiscoverShows()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing shows response")
}

func TestNTS_MalformedJSON_FetchPlaylist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{invalid json")
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	_, err := provider.FetchPlaylist("show/ep")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing tracklist response")
}

// =============================================================================
// Provider Registration Test
// =============================================================================

func TestNTS_ProviderRegistration(t *testing.T) {
	service := &RadioService{}

	// NTS should be registered
	provider, err := service.getProvider(models.PlaylistSourceNTS)
	require.NoError(t, err)
	assert.NotNil(t, provider)

	// Should return an NTSProvider
	ntsProvider, ok := provider.(*NTSProvider)
	assert.True(t, ok, "should return an NTSProvider instance")
	assert.NotNil(t, ntsProvider)
	defer ntsProvider.Close()

	// Unsupported source should error
	_, err = service.getProvider("unsupported_api")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported playlist source")
}

// =============================================================================
// User-Agent Test
// =============================================================================

func TestNTS_UserAgent(t *testing.T) {
	var capturedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, ntsEmptyShowsJSON)
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	_, _ = provider.DiscoverShows()
	assert.Equal(t, ntsUserAgent, capturedUA, "should send correct User-Agent header")
}

// =============================================================================
// Close Test
// =============================================================================

func TestNTS_Close(t *testing.T) {
	provider := NewNTSProvider()
	assert.NotPanics(t, func() {
		provider.Close()
	})

	// Close again should not panic
	assert.NotPanics(t, func() {
		provider.Close()
	})
}

// =============================================================================
// parseNTSShow Unit Tests
// =============================================================================

func TestNTS_ParseNTSShow(t *testing.T) {
	// Show with all fields
	show := parseNTSShow(ntsShow{
		Name:        "Test Show",
		Alias:       "test-show",
		Description: "A test show",
		ImageURL:    "https://example.com/image.jpg",
		Hosts:       []ntsHost{{Name: "Host A"}, {Name: "Host B"}},
	})

	assert.Equal(t, "test-show", show.ExternalID)
	assert.Equal(t, "Test Show", show.Name)
	require.NotNil(t, show.Description)
	assert.Equal(t, "A test show", *show.Description)
	require.NotNil(t, show.HostName)
	assert.Equal(t, "Host A, Host B", *show.HostName)
	require.NotNil(t, show.ImageURL)
	assert.Equal(t, "https://example.com/image.jpg", *show.ImageURL)
	require.NotNil(t, show.ArchiveURL)
	assert.Equal(t, "https://www.nts.live/shows/test-show", *show.ArchiveURL)

	// Show with minimal fields
	minShow := parseNTSShow(ntsShow{
		Name:  "Minimal",
		Alias: "minimal",
	})
	assert.Equal(t, "minimal", minShow.ExternalID)
	assert.Equal(t, "Minimal", minShow.Name)
	assert.Nil(t, minShow.Description)
	assert.Nil(t, minShow.HostName)
	assert.Nil(t, minShow.ImageURL)
	require.NotNil(t, minShow.ArchiveURL)
}

// =============================================================================
// parseNTSEpisode Unit Tests
// =============================================================================

func TestNTS_ParseNTSEpisode(t *testing.T) {
	// Full episode
	ep := parseNTSEpisode(ntsEpisode{
		Name:            "Show - March 2026",
		EpisodeAlias:    "march-2026",
		Broadcast:       "2026-03-15T20:00:00Z",
		Mixcloud:        "https://www.mixcloud.com/NTSRadio/show-march-2026/",
		DurationMinutes: 120,
	}, "test-show")

	assert.Equal(t, "test-show/march-2026", ep.ExternalID)
	assert.Equal(t, "test-show", ep.ShowExternalID)
	assert.Equal(t, "2026-03-15", ep.AirDate)
	require.NotNil(t, ep.AirTime)
	assert.Equal(t, "20:00:00", *ep.AirTime)
	require.NotNil(t, ep.Title)
	assert.Equal(t, "Show - March 2026", *ep.Title)
	require.NotNil(t, ep.ArchiveURL)
	assert.Contains(t, *ep.ArchiveURL, "mixcloud.com")
	require.NotNil(t, ep.DurationMinutes)
	assert.Equal(t, 120, *ep.DurationMinutes)

	// Episode with no mixcloud, no duration
	minEp := parseNTSEpisode(ntsEpisode{
		Name:         "Minimal Episode",
		EpisodeAlias: "min-ep",
		Broadcast:    "2026-01-01T10:00:00Z",
	}, "show")

	assert.Equal(t, "show/min-ep", minEp.ExternalID)
	assert.Nil(t, minEp.ArchiveURL)
	assert.Nil(t, minEp.DurationMinutes)
}

func TestNTS_ParseNTSEpisode_DateOnlyBroadcast(t *testing.T) {
	ep := parseNTSEpisode(ntsEpisode{
		Name:         "Date Only",
		EpisodeAlias: "date-only",
		Broadcast:    "2026-03-15",
	}, "show")

	assert.Equal(t, "2026-03-15", ep.AirDate)
	require.NotNil(t, ep.AirTime)
	assert.Equal(t, "00:00:00", *ep.AirTime)
}
