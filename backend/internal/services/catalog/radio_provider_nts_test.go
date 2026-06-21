package catalog

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Test fixtures — realistic NTS API JSON responses
// =============================================================================

const ntsShowsPage1JSON = `{
  "results": [
    {
      "name": "Huerco S.",
      "show_alias": "huerco-s",
      "description": "Kansas-born ambient producer Brian Leeds presents a monthly show exploring the fringes of experimental music.",
      "media": {"picture_large": "https://media.nts.live/uploads/huerco-s.jpg"}
    },
    {
      "name": "Scratcha DVA",
      "show_alias": "scratcha-dva",
      "description": "London DJ and producer Scratcha DVA mixing bass, grime, and club music from the diaspora.",
      "media": {"picture_large": "https://media.nts.live/uploads/scratcha-dva.jpg"}
    },
    {
      "name": "Morning Becomes Eclectic",
      "show_alias": "morning-becomes-eclectic",
      "description": "An eclectic morning show with no singular host, featuring guest DJs and live sessions.",
      "media": {}
    }
  ]
}`

const ntsShowsPage2JSON = `{
  "results": [
    {
      "name": "Donato Dozzy",
      "show_alias": "donato-dozzy",
      "description": "Legendary Italian techno producer showcasing hypnotic rhythms and deep electronics.",
      "media": {"picture_large": "https://media.nts.live/uploads/donato-dozzy.jpg"}
    }
  ]
}`

const ntsEpisodesJSON = `{
  "results": [
    {
      "name": "Huerco S. - March 2026",
      "episode_alias": "march-2026",
      "broadcast": "2026-03-15T20:00:00+00:00",
      "mixcloud": "https://www.mixcloud.com/NTSRadio/huerco-s-15th-march-2026/"
    },
    {
      "name": "Huerco S. - February 2026",
      "episode_alias": "february-2026",
      "broadcast": "2026-02-15T20:00:00+00:00",
      "mixcloud": "https://www.mixcloud.com/NTSRadio/huerco-s-15th-february-2026/"
    },
    {
      "name": "Huerco S. - January 2026",
      "episode_alias": "january-2026",
      "broadcast": "2026-01-15T20:00:00+00:00",
      "mixcloud": "https://www.mixcloud.com/NTSRadio/huerco-s-15th-january-2026/"
    }
  ]
}`

const ntsEpisodeOlderJSON = `{
  "results": [
    {
      "name": "Huerco S. - December 2025",
      "episode_alias": "december-2025",
      "broadcast": "2025-12-15T20:00:00+00:00",
      "mixcloud": "https://www.mixcloud.com/NTSRadio/huerco-s-15th-december-2025/"
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
		_, _ = fmt.Fprint(w, ntsShowsPage1JSON)
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
	assert.Nil(t, h.HostName, "NTS /v2/shows exposes no host field")
	require.NotNil(t, h.ImageURL)
	assert.Equal(t, "https://media.nts.live/uploads/huerco-s.jpg", *h.ImageURL)
	require.NotNil(t, h.ArchiveURL)
	assert.Equal(t, "https://www.nts.live/shows/huerco-s", *h.ArchiveURL)

	// Check Scratcha DVA
	s := shows[1]
	assert.Equal(t, "scratcha-dva", s.ExternalID)
	assert.Equal(t, "Scratcha DVA", s.Name)
	assert.Nil(t, s.HostName)

	// Check Morning Becomes Eclectic — no image (empty media)
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
			_, _ = w.Write(data)
		} else {
			// Second page: return fewer results (signals end of pagination)
			_, _ = fmt.Fprint(w, ntsShowsPage2JSON)
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
		_, _ = fmt.Fprint(w, ntsEmptyShowsJSON)
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
		_, _ = fmt.Fprint(w, ntsEpisodesJSON)
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
}

func TestNTS_FetchNewEpisodes_DateFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, ntsEpisodesJSON)
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
			_, _ = w.Write(data)
		} else {
			// Second page has old episodes
			_, _ = fmt.Fprint(w, ntsEpisodeOlderJSON)
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
		_, _ = fmt.Fprint(w, ntsEmptyEpisodesJSON)
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
		_, _ = fmt.Fprint(w, ntsTracklistJSON)
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
	// PSY-1143: NTS has no stable provider play id, so dedup falls back to the
	// content hash. ProviderPlayID must stay nil.
	assert.Nil(t, p0.ProviderPlayID)

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
		_, _ = fmt.Fprint(w, ntsEmptyTracklistJSON)
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
		_, _ = fmt.Fprint(w, `{"detail": "Not found."}`)
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
		_, _ = fmt.Fprint(w, tracklistJSON)
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
// Mixcloud Archive URL Tests
// =============================================================================

func TestNTS_MixcloudArchiveURL_Preserved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, ntsEpisodesJSON)
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
		_, _ = fmt.Fprint(w, ntsEmptyShowsJSON)
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
		_, _ = fmt.Fprint(w, "Internal Server Error")
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
		_, _ = fmt.Fprint(w, "Not Found")
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
		_, _ = fmt.Fprint(w, "Service Unavailable")
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
		_, _ = fmt.Fprint(w, "this is not json")
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
		_, _ = fmt.Fprint(w, "{invalid json")
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
	provider, err := service.getProvider(catalogm.PlaylistSourceNTS)
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
		_, _ = fmt.Fprint(w, ntsEmptyShowsJSON)
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
		Media:       ntsMedia{PictureLarge: "https://example.com/image.jpg"},
	})

	assert.Equal(t, "test-show", show.ExternalID)
	assert.Equal(t, "Test Show", show.Name)
	require.NotNil(t, show.Description)
	assert.Equal(t, "A test show", *show.Description)
	assert.Nil(t, show.HostName, "NTS /v2/shows exposes no host field")
	require.NotNil(t, show.ImageURL)
	assert.Equal(t, "https://example.com/image.jpg", *show.ImageURL)
	require.NotNil(t, show.ArchiveURL)
	assert.Equal(t, "https://www.nts.live/shows/test-show", *show.ArchiveURL)

	// Image falls back to background_large when picture_large is absent
	bgShow := parseNTSShow(ntsShow{
		Name:  "Bg",
		Alias: "bg",
		Media: ntsMedia{BackgroundLarge: "https://example.com/bg.jpg"},
	})
	require.NotNil(t, bgShow.ImageURL)
	assert.Equal(t, "https://example.com/bg.jpg", *bgShow.ImageURL)

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
		Name:         "Show - March 2026",
		EpisodeAlias: "march-2026",
		Broadcast:    "2026-03-15T20:00:00Z",
		Mixcloud:     "https://www.mixcloud.com/NTSRadio/show-march-2026/",
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

	// Episode with no mixcloud
	minEp := parseNTSEpisode(ntsEpisode{
		Name:         "Minimal Episode",
		EpisodeAlias: "min-ep",
		Broadcast:    "2026-01-01T10:00:00Z",
	}, "show")

	assert.Equal(t, "show/min-ep", minEp.ExternalID)
	assert.Nil(t, minEp.ArchiveURL)
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

func TestNTS_ParseNTSEpisode_DerivesDateFromAliasWhenNoBroadcast(t *testing.T) {
	// Older NTS episodes return no `broadcast`; the air date must be recovered
	// from the alias so the episode still imports (air_date is NOT NULL).
	ep := parseNTSEpisode(ntsEpisode{
		Name:         "Anu",
		EpisodeAlias: "anu-11th-july-2017",
	}, "anu")

	assert.Equal(t, "anu/anu-11th-july-2017", ep.ExternalID)
	assert.Equal(t, "2017-07-11", ep.AirDate, "should derive air date from the alias")
	assert.Nil(t, ep.AirTime, "no broadcast means no air time")
}

func TestNTS_ParseNTSEpisode_NoRecoverableDate(t *testing.T) {
	// No broadcast and an alias without a trailing date -> AirDate stays empty.
	// The import layer skips such episodes rather than insert an invalid date.
	ep := parseNTSEpisode(ntsEpisode{
		Name:         "Pilot",
		EpisodeAlias: "pilot-episode",
	}, "show")

	assert.Empty(t, ep.AirDate)
	assert.Nil(t, ep.AirTime)
}

func TestNTS_DateFromNTSAlias(t *testing.T) {
	cases := []struct {
		name  string
		alias string
		want  string
	}{
		{"th ordinal", "anu-11th-july-2017", "2017-07-11"},
		{"st ordinal", "anu-21st-march-2017", "2017-03-21"},
		{"nd ordinal", "show-2nd-june-2020", "2020-06-02"},
		{"rd ordinal", "show-3rd-may-2019", "2019-05-03"},
		{"multi-word slug", "anu-jm-moser-lil-stoner-diva-18th-april-2017", "2017-04-18"},
		{"single-digit day", "x-1st-january-2021", "2021-01-01"},
		{"mixed case", "Anu-11TH-July-2017", "2017-07-11"},
		{"no date in slug", "morning-becomes-eclectic", ""},
		{"month-year only, no day", "huerco-s-march-2026", ""},
		{"invalid month name", "x-10th-smarch-2020", ""},
		{"impossible date", "x-31st-february-2020", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, dateFromNTSAlias(tc.alias))
		})
	}
}

func TestNTS_ParseNTSBroadcast(t *testing.T) {
	cases := []struct {
		name             string
		broadcast        string
		wantOK           bool
		wantDate, wantTm string
	}{
		{"rfc3339 numeric offset (real NTS format)", "2021-11-04T12:00:00+00:00", true, "2021-11-04", "12:00:00"},
		{"rfc3339 with Z", "2026-03-15T20:00:00Z", true, "2026-03-15", "20:00:00"},
		{"non-UTC offset", "2026-05-20T19:00:18-07:00", true, "2026-05-20", "19:00:18"},
		{"date only", "2026-03-15", true, "2026-03-15", "00:00:00"},
		{"empty", "", false, "", ""},
		{"garbage", "not-a-date", false, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseNTSBroadcast(tc.broadcast)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantDate, got.Format("2006-01-02"))
				assert.Equal(t, tc.wantTm, got.Format("15:04:05"))
			}
		})
	}
}

func TestNTS_ParseNTSEpisode_OffsetBroadcast(t *testing.T) {
	// Regression: NTS returns RFC3339 with a numeric offset, which the old
	// literal-Z layout failed to parse — dropping every episode's air date.
	ep := parseNTSEpisode(ntsEpisode{
		Name:         "Anu",
		EpisodeAlias: "anu-4th-november-2021",
		Broadcast:    "2021-11-04T12:00:00+00:00",
	}, "anu")

	assert.Equal(t, "2021-11-04", ep.AirDate)
	require.NotNil(t, ep.AirTime)
	assert.Equal(t, "12:00:00", *ep.AirTime)
}

func TestNTS_FetchNewEpisodes_DateFiltering_OffsetFormat(t *testing.T) {
	// Regression: the [since, until] filter parsed broadcast with a literal-Z
	// layout, so the real "+00:00" offset format never parsed and the window
	// silently never applied (every episode passed through).
	const body = `{"results":[
		{"name":"Nov","episode_alias":"show-4th-november-2021","broadcast":"2021-11-04T12:00:00+00:00"},
		{"name":"Oct","episode_alias":"show-20th-october-2021","broadcast":"2021-10-20T13:00:00+00:00"},
		{"name":"Sep","episode_alias":"show-8th-september-2021","broadcast":"2021-09-08T13:00:00+00:00"}
	]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, body)
	}))
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	// Only episodes on/after Oct 1, 2021: the Sep episode is filtered out (and,
	// since results are date-descending, the walk stops there).
	since := time.Date(2021, 10, 1, 0, 0, 0, 0, time.UTC)
	episodes, err := provider.FetchNewEpisodes("show", since, time.Time{})
	require.NoError(t, err)
	require.Len(t, episodes, 2)
	assert.Equal(t, "2021-11-04", episodes[0].AirDate)
	assert.Equal(t, "2021-10-20", episodes[1].AirDate)
}
