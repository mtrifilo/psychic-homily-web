package catalog

// Live now-playing adapter tests (PSY-1022). All provider parsing runs
// against CANNED fixtures in ./testdata — snapshots of the real provider
// responses taken 2026-06-11 — served from httptest servers. Tests never hit
// live provider APIs.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadLiveTestdata reads a fixture file from ./testdata.
func loadLiveTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err, "reading testdata file %s", name)
	return data
}

// =============================================================================
// KEXP
// =============================================================================

func newKEXPLiveServer(t *testing.T, playsFixture string) *httptest.Server {
	t.Helper()
	showsBody := loadLiveTestdata(t, "kexp_live_shows.json")
	playsBody := loadLiveTestdata(t, playsFixture)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v2/shows/"):
			_, _ = w.Write(showsBody)
		case strings.HasPrefix(r.URL.Path, "/v2/plays/"):
			_, _ = w.Write(playsBody)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestKEXPFetchLiveNowPlaying(t *testing.T) {
	server := newKEXPLiveServer(t, "kexp_live_plays.json")
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	live, err := provider.FetchLiveNowPlaying("")
	require.NoError(t, err)
	require.NotNil(t, live)

	assert.Equal(t, "The Morning Show", live.ShowName)
	require.NotNil(t, live.ShowExternalID)
	assert.Equal(t, "16", *live.ShowExternalID)
	require.NotNil(t, live.HostName)
	assert.Equal(t, "John Richards", *live.HostName)

	require.NotNil(t, live.CurrentTrack)
	assert.Equal(t, "Diana Ross", live.CurrentTrack.ArtistName)
	require.NotNil(t, live.CurrentTrack.TrackTitle)
	assert.Equal(t, "I’m Coming Out", *live.CurrentTrack.TrackTitle)
	require.NotNil(t, live.CurrentTrack.AlbumTitle)
	assert.Equal(t, "I'm Coming Out / Give Up", *live.CurrentTrack.AlbumTitle)
	require.NotNil(t, live.CurrentTrack.RotationStatus)
	assert.Equal(t, "Library", *live.CurrentTrack.RotationStatus)
	require.NotNil(t, live.CurrentTrack.MusicBrainzArtistID)
	assert.Equal(t, "60d41417-feda-4734-bbbf-7dcc30e08a83", *live.CurrentTrack.MusicBrainzArtistID)

	// The airbreak in position 2 is skipped; remaining trackplays become
	// the recent history in feed order (most recent first).
	require.Len(t, live.RecentTracks, 5)
	assert.Equal(t, "Chic", live.RecentTracks[0].ArtistName)
	assert.Equal(t, "Diana Ross", live.RecentTracks[1].ArtistName)
}

func TestKEXPFetchLiveNowPlaying_AirbreakHead(t *testing.T) {
	// When the newest play is an airbreak, KEXP is between tracks: no
	// current track, but the prior trackplays still feed the recents.
	server := newKEXPLiveServer(t, "kexp_live_plays_airbreak.json")
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	live, err := provider.FetchLiveNowPlaying("")
	require.NoError(t, err)
	require.NotNil(t, live)
	assert.Equal(t, "The Morning Show", live.ShowName)
	assert.Nil(t, live.CurrentTrack)
	require.Len(t, live.RecentTracks, 1)
	assert.Equal(t, "Chic", live.RecentTracks[0].ArtistName)
}

func TestKEXPFetchLiveNowPlaying_ProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	provider := NewKEXPProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	live, err := provider.FetchLiveNowPlaying("")
	require.Error(t, err)
	assert.Nil(t, live)
}

// =============================================================================
// NTS
// =============================================================================

func newNTSLiveServer(t *testing.T) *httptest.Server {
	t.Helper()
	body := loadLiveTestdata(t, "nts_live.json")
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/live" {
			_, _ = w.Write(body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestNTSFetchLiveNowPlaying(t *testing.T) {
	server := newNTSLiveServer(t)
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	live, err := provider.FetchLiveNowPlaying("1")
	require.NoError(t, err)
	require.NotNil(t, live)
	// The embedded details name (proper case) wins over the ALL-CAPS
	// broadcast_title; the show alias rides along for external-id matching.
	assert.Equal(t, "Maï-Linh", live.ShowName)
	require.NotNil(t, live.ShowExternalID)
	assert.Equal(t, "mai-linh", *live.ShowExternalID)
	// NTS live is show-level only.
	assert.Nil(t, live.CurrentTrack)
	assert.Empty(t, live.RecentTracks)

	live2, err := provider.FetchLiveNowPlaying("2")
	require.NoError(t, err)
	require.NotNil(t, live2)
	assert.Equal(t, "The Outsiders w/ Rich Tupica - The Cramps Special", live2.ShowName)
	require.NotNil(t, live2.ShowExternalID)
	assert.Equal(t, "outsider-oldies", *live2.ShowExternalID)
}

func TestNTSFetchLiveNowPlaying_UnknownChannel(t *testing.T) {
	server := newNTSLiveServer(t)
	defer server.Close()

	provider := NewNTSProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	live, err := provider.FetchLiveNowPlaying("3")
	require.NoError(t, err)
	assert.Nil(t, live)
}

// =============================================================================
// WFMU
// =============================================================================

func newWFMULiveServer(t *testing.T, fixture string) *httptest.Server {
	t.Helper()
	return newWFMULiveServerRaw(t, loadLiveTestdata(t, fixture))
}

// newWFMULiveServerRaw serves the given aggregator bytes on the live-now-playing
// path (newWFMULiveServer is the testdata-file variant); both back the WFMU
// FetchLiveNowPlaying tests.
func newWFMULiveServerRaw(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/currentliveshows_aggregator.php" {
			_, _ = w.Write(body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestWFMUFetchLiveNowPlaying_AllChannels(t *testing.T) {
	server := newWFMULiveServer(t, "wfmu_currentliveshows.html")
	defer server.Close()

	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	// Main stream: plain `"Title" by Artist` bigline.
	main, err := provider.FetchLiveNowPlaying(wfmuLiveChannelMain)
	require.NoError(t, err)
	require.NotNil(t, main)
	assert.Equal(t, "Push Button Heaven", main.ShowName)
	require.NotNil(t, main.HostName)
	assert.Equal(t, "Jody Peyote", *main.HostName)
	require.NotNil(t, main.ShowExternalID)
	assert.Equal(t, "P2", *main.ShowExternalID)
	require.NotNil(t, main.CurrentTrack)
	assert.Equal(t, "david a jaycock", main.CurrentTrack.ArtistName)
	require.NotNil(t, main.CurrentTrack.TrackTitle)
	assert.Equal(t, "Circling the Church", *main.CurrentTrack.TrackTitle)

	// Drummer: title contains an unbalanced parenthesis — stays inside the
	// quoted group.
	drummer, err := provider.FetchLiveNowPlaying(wfmuLiveChannelDrummer)
	require.NoError(t, err)
	require.NotNil(t, drummer)
	assert.Equal(t, "Wound Liquor", drummer.ShowName)
	require.NotNil(t, drummer.HostName)
	assert.Equal(t, "Olleh", *drummer.HostName)
	require.NotNil(t, drummer.ShowExternalID)
	assert.Equal(t, "WQ", *drummer.ShowExternalID)
	require.NotNil(t, drummer.CurrentTrack)
	assert.Equal(t, "Fathom", drummer.CurrentTrack.ArtistName)

	// Rock'n'Soul: "Your DJ speaks over" prefix is stripped from the track.
	rocknsoul, err := provider.FetchLiveNowPlaying(wfmuLiveChannelRockSoul)
	require.NoError(t, err)
	require.NotNil(t, rocknsoul)
	assert.Equal(t, "Soul Lunch", rocknsoul.ShowName)
	require.NotNil(t, rocknsoul.HostName)
	assert.Equal(t, "Soul'n'Soul Bunny", *rocknsoul.HostName)
	require.NotNil(t, rocknsoul.ShowExternalID)
	assert.Equal(t, "SR", *rocknsoul.ShowExternalID)
	require.NotNil(t, rocknsoul.CurrentTrack)
	assert.Equal(t, "TSU Tornados", rocknsoul.CurrentTrack.ArtistName)
	require.NotNil(t, rocknsoul.CurrentTrack.TrackTitle)
	assert.Equal(t, "Getting the Corners", *rocknsoul.CurrentTrack.TrackTitle)

	// Sheena's Jungle Room.
	sheena, err := provider.FetchLiveNowPlaying(wfmuLiveChannelSheena)
	require.NoError(t, err)
	require.NotNil(t, sheena)
	assert.Equal(t, "Vortex of Chaos", sheena.ShowName)
	require.NotNil(t, sheena.ShowExternalID)
	assert.Equal(t, "VC", *sheena.ShowExternalID)
	require.NotNil(t, sheena.CurrentTrack)
	assert.Equal(t, "Madder Mortem", sheena.CurrentTrack.ArtistName)
	require.NotNil(t, sheena.CurrentTrack.TrackTitle)
	assert.Equal(t, "Resolution", *sheena.CurrentTrack.TrackTitle)
}

// wfmuAggregatorNoDrummerDJHTML is a reduced aggregator fragment where the
// Give the Drummer Radio block has NO playlist link — WFMU's signal that the
// stream is looping unattended (no live DJ). Side streams without a playlist
// link must report not-live so the service serves the archive fallback.
const wfmuAggregatorNoDrummerDJHTML = `
<div id="nowplaysong">
<div id="nowplaying">
<div class="item-even">
<div class="streamtitle">
WFMU stream
(<a href="https://wfmu.org/table">Schedule</a>)
</div>
<div class="bigline">
&quot;Circling the Church&quot;
by
david a jaycock
</div>
<div class="smallline">
<span class="KDBFavIcon KDBprogram" id="KDBprogram-P2"><a href="#">x</a></span>
on Push Button Heaven with Jody Peyote
</div>
<div class="linkssection">
<span class="links">
<a href="/playlists/shows/165181">Playlist &amp; Comments</a>
</span>
</div>
</div>
<div class="item-odd">
<div class="streamtitle">
Give the Drummer Radio stream
(<a href="https://wfmu.org/drummer">Schedule</a>)
</div>
<div class="bigline">
&quot;Some Archived Tune&quot;
by
Somebody
</div>
<div class="smallline">
on Some Rerun Show with Someone
</div>
<div class="linkssection">
<span class="links">
<a href="/wfmu_drummer.pls">128k MP3</a>
</span>
</div>
</div>
</div>
</div>`

func TestWFMUFetchLiveNowPlaying_NoLiveDJOnSideStream(t *testing.T) {
	server := newWFMULiveServerRaw(t, []byte(wfmuAggregatorNoDrummerDJHTML))
	defer server.Close()

	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	// Side stream without a playlist link → not live.
	drummer, err := provider.FetchLiveNowPlaying(wfmuLiveChannelDrummer)
	require.NoError(t, err)
	assert.Nil(t, drummer)

	// Main stream is live here because its block carries a playlist link (a live
	// DJ logging tracks), not because main is exempt from the rule (PSY-1239).
	main, err := provider.FetchLiveNowPlaying(wfmuLiveChannelMain)
	require.NoError(t, err)
	require.NotNil(t, main)
	assert.Equal(t, "Push Button Heaven", main.ShowName)
}

// wfmuAggregatorNoLiveMainHTML is an aggregator fragment where the MAIN 91.1
// block has NO playlist link — the stream is looping unattended (automation, or
// a rebroadcast). Pre-PSY-1239 the main stream was exempt and reported "ON AIR"
// with whatever show the widget named (e.g. a rebroadcast); now it must report
// not-live so the service serves the latest-archive fallback.
const wfmuAggregatorNoLiveMainHTML = `
<div id="nowplaysong">
<div id="nowplaying">
<div class="item-even">
<div class="streamtitle">
WFMU stream
(<a href="https://wfmu.org/table">Schedule</a>)
</div>
<div class="bigline">
&quot;Some Old Tune&quot;
by
A Rerun Artist
</div>
<div class="smallline">
on Burn It Down! with Some DJ
</div>
<div class="linkssection">
<span class="links">
<a href="/wfmu.pls">128k MP3</a>
</span>
</div>
</div>
</div>
</div>`

func TestWFMUFetchLiveNowPlaying_NoLiveDJOnMain(t *testing.T) {
	server := newWFMULiveServerRaw(t, []byte(wfmuAggregatorNoLiveMainHTML))
	defer server.Close()

	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	// Main stream with no playlist link → not live (PSY-1239); the unattended
	// stream must NOT surface as "ON AIR" — the caller falls back to the archive.
	main, err := provider.FetchLiveNowPlaying(wfmuLiveChannelMain)
	require.NoError(t, err)
	assert.Nil(t, main)
}

func TestParseWFMUSmallline(t *testing.T) {
	tests := []struct {
		in       string
		wantShow string
		wantHost string // "" = nil
	}{
		{"on Push Button Heaven with Jody Peyote", "Push Button Heaven", "Jody Peyote"},
		{"on Sinner's Crossroads with Kevin Nutt", "Sinner's Crossroads", "Kevin Nutt"},
		// Show names containing "with" keep everything before the LAST "with".
		{"on Dancing with Tears with DJ X", "Dancing with Tears", "DJ X"},
		{"on Solo Show", "Solo Show", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		show, host := parseWFMUSmallline(tt.in)
		assert.Equal(t, tt.wantShow, show, "show for %q", tt.in)
		if tt.wantHost == "" {
			assert.Nil(t, host, "host for %q", tt.in)
		} else {
			require.NotNil(t, host, "host for %q", tt.in)
			assert.Equal(t, tt.wantHost, *host, "host for %q", tt.in)
		}
	}
}
