package catalog

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Test fixtures — realistic WFMU HTML/RSS content
// =============================================================================

const wfmuDJIndexHTML = `<!DOCTYPE html>
<html>
<head><title>WFMU Playlists Index</title></head>
<body>
<h2>Monday</h2>
<p>
<b>Wake</b> with Trouble -
<a href="/playlists/WA">playlists and archives</a>
[ RSS feeds: <a href="/archivefeed/mp3/WA.xml">MP3</a> | <a href="/playlistfeed/WA.xml">Playlists</a> ]
</p>
<p>
<b>Surface Noise</b> with Dave the Spade -
<a href="/playlists/SN">playlists and archives</a>
[ RSS feeds: <a href="/archivefeed/mp3/SN.xml">MP3</a> | <a href="/playlistfeed/SN.xml">Playlists</a> ]
</p>
<h2>Tuesday</h2>
<p>
<b>Nervous Boogie</b> with Richard J. -
<a href="/playlists/NB">playlists and archives</a>
[ RSS feeds: <a href="/archivefeed/mp3/NB.xml">MP3</a> | <a href="/playlistfeed/NB.xml">Playlists</a> ]
</p>
<h2>The Rest</h2>
<p>
<b>Shmelting</b> with Zabedon -
<a href="/playlists/S5">playlists and archives</a>
[ RSS feeds: <a href="/archivefeed/mp3/S5.xml">MP3</a> | <a href="/playlistfeed/S5.xml">Playlists</a> ]
</p>
</body>
</html>`

const wfmuRSSFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
  <title>WFMU's recent playlists from Nervous Boogie with Richard J.</title>
  <link>http://www.wfmu.org/playlists/NB</link>
  <description>WFMU's most recent playlists from Nervous Boogie with Richard J.</description>
  <language>en</language>
  <item>
    <title>WFMU Playlist: Nervous Boogie from March 12, 2026</title>
    <link>http://www.wfmu.org/playlists/shows/162145</link>
    <description>&lt;a href="http://www.wfmu.org/playlists/shows/162145"&gt;Playlist&lt;/a&gt; from Nervous Boogie on WFMU</description>
    <guid isPermaLink="true">http://www.wfmu.org/playlists/shows/162145</guid>
    <pubDate>Thu, 12 Mar 2026 22:00:00 -0400</pubDate>
  </item>
  <item>
    <title>WFMU Playlist: Nervous Boogie from March 5, 2026</title>
    <link>http://www.wfmu.org/playlists/shows/161980</link>
    <description>&lt;a href="http://www.wfmu.org/playlists/shows/161980"&gt;Playlist&lt;/a&gt; from Nervous Boogie on WFMU</description>
    <guid isPermaLink="true">http://www.wfmu.org/playlists/shows/161980</guid>
    <pubDate>Thu, 05 Mar 2026 22:00:00 -0400</pubDate>
  </item>
  <item>
    <title>WFMU Playlist: Nervous Boogie from February 26, 2026</title>
    <link>http://www.wfmu.org/playlists/shows/161800</link>
    <description>&lt;a href="http://www.wfmu.org/playlists/shows/161800"&gt;Playlist&lt;/a&gt;</description>
    <guid isPermaLink="true">http://www.wfmu.org/playlists/shows/161800</guid>
    <pubDate>Thu, 26 Feb 2026 22:00:00 -0400</pubDate>
  </item>
  <item>
    <title>WFMU Playlist: Nervous Boogie from January 15, 2026</title>
    <link>http://www.wfmu.org/playlists/shows/160500</link>
    <description>&lt;a href="http://www.wfmu.org/playlists/shows/160500"&gt;Playlist&lt;/a&gt;</description>
    <guid isPermaLink="true">http://www.wfmu.org/playlists/shows/160500</guid>
    <pubDate>Thu, 15 Jan 2026 22:00:00 -0400</pubDate>
  </item>
</channel>
</rss>`

const wfmuPlaylistPageHTML = `<!DOCTYPE html>
<html>
<head><title>Nervous Boogie: Playlist from March 12, 2026</title></head>
<body>
<h1>Nervous Boogie with Richard J.: Playlist from March 12, 2026</h1>
<table class="showlist">
<thead>
<tr class="head"><th>Artist</th><th>Track</th><th>Album</th><th>Label</th><th>Year</th><th>Format</th><th>Comments</th><th>Images</th><th>New</th><th>Approx. start time</th></tr>
</thead>
<tbody>
<tr>
  <td>Sir Lattimore Brown</td>
  <td>Shake and Vibrate</td>
  <td></td>
  <td>Sound Stage 7</td>
  <td></td>
  <td>7"</td>
  <td></td>
  <td></td>
  <td></td>
  <td>0:00:15</td>
</tr>
<tr>
  <td>The V.I.P.'s</td>
  <td>Flashback</td>
  <td></td>
  <td>Bigtop Records</td>
  <td></td>
  <td>7"</td>
  <td>Orlons sound!</td>
  <td></td>
  <td></td>
  <td>0:02:15</td>
</tr>
<tr>
  <td>Ramona King</td>
  <td>It Couldn't Happen To A Nicer Guy</td>
  <td></td>
  <td>Warner Bros. Records</td>
  <td>1964</td>
  <td>7"</td>
  <td></td>
  <td></td>
  <td></td>
  <td>0:05:30</td>
</tr>
<tr>
  <td>The Front Page &amp; Her</td>
  <td>He's Groovy</td>
  <td>Hearts For Sale! Girl Group Sounds USA 1961-1967</td>
  <td>Ace Records Ltd.</td>
  <td>2022</td>
  <td>LP</td>
  <td>Fantastic comp of girl group rarities</td>
  <td><img src="/images/album123.jpg" alt="album art"/></td>
  <td>New</td>
  <td>0:09:03</td>
</tr>
<tr>
  <td>Ann Peebles</td>
  <td>I Can't Stand The Rain</td>
  <td></td>
  <td>Hi Records</td>
  <td>1973</td>
  <td>7"</td>
  <td>Stone cold classic</td>
  <td></td>
  <td></td>
  <td>0:12:45</td>
</tr>
<tr>
  <td>Curtis Mayfield</td>
  <td>If I Were Only A Child Again</td>
  <td></td>
  <td>Curtom</td>
  <td>1974</td>
  <td>7"</td>
  <td></td>
  <td></td>
  <td></td>
  <td>0:16:00</td>
</tr>
<tr>
  <td>Jody Reynolds</td>
  <td>Tarantula</td>
  <td>They Move in the Night</td>
  <td>Numero Group</td>
  <td>2023</td>
  <td>LP</td>
  <td>Amazing reissue from Numero. DJ Pick of the week!</td>
  <td><img src="/images/album456.jpg" alt="album art"/></td>
  <td><img src="/Gfx/new_icon.gif" alt="new release"/></td>
  <td>0:19:30</td>
</tr>
</tbody>
</table>
</body>
</html>`

const wfmuPlaylistWithPromosHTML = `<!DOCTYPE html>
<html>
<body>
<table class="showlist">
<thead><tr class="head"><th>Artist</th><th>Track</th><th>Album</th><th>Label</th><th>Year</th><th>Format</th><th>Comments</th><th>Images</th><th>New</th><th>Start</th></tr></thead>
<tbody>
<tr>
  <td>The Sonics</td>
  <td>Psycho</td>
  <td></td>
  <td>Etiquette</td>
  <td>1965</td>
  <td>7"</td>
  <td></td>
  <td></td>
  <td></td>
  <td>0:00:15</td>
</tr>
<tr>
  <td>WFMU</td>
  <td>Pledge Drive Spot</td>
  <td></td>
  <td></td>
  <td></td>
  <td></td>
  <td>Please donate to keep WFMU alive!</td>
  <td></td>
  <td></td>
  <td>0:03:00</td>
</tr>
<tr>
  <td>Station ID</td>
  <td>WFMU 91.1 FM</td>
  <td></td>
  <td></td>
  <td></td>
  <td></td>
  <td></td>
  <td></td>
  <td></td>
  <td>0:04:00</td>
</tr>
<tr>
  <td>Link Wray</td>
  <td>Rumble</td>
  <td></td>
  <td>Cadence</td>
  <td>1958</td>
  <td>7"</td>
  <td>The greatest instrumental ever recorded</td>
  <td></td>
  <td></td>
  <td>0:04:30</td>
</tr>
<tr>
  <td>Various</td>
  <td>Fundraiser Promo</td>
  <td></td>
  <td></td>
  <td></td>
  <td></td>
  <td>Help us reach our goal!</td>
  <td></td>
  <td></td>
  <td>0:08:00</td>
</tr>
<tr>
  <td>The Cramps</td>
  <td>Human Fly</td>
  <td></td>
  <td>Vengeance</td>
  <td>1978</td>
  <td>7"</td>
  <td></td>
  <td></td>
  <td></td>
  <td>0:08:30</td>
</tr>
</tbody>
</table>
</body>
</html>`

const wfmuEmptyPlaylistHTML = `<!DOCTYPE html>
<html>
<body>
<h1>Sample Show: Playlist from March 1, 2026</h1>
<table class="showlist">
<thead><tr class="head"><th>Artist</th><th>Track</th><th>Album</th><th>Label</th><th>Year</th><th>Format</th><th>Comments</th><th>Images</th><th>New</th><th>Start</th></tr></thead>
<tbody>
</tbody>
</table>
</body>
</html>`

const wfmuMinimalColumnsHTML = `<!DOCTYPE html>
<html>
<body>
<table class="showlist">
<tbody>
<tr><td>The Stooges</td><td>I Wanna Be Your Dog</td><td>The Stooges</td></tr>
<tr><td>MC5</td><td>Kick Out the Jams</td><td>Kick Out the Jams</td></tr>
</tbody>
</table>
</body>
</html>`

const wfmuRSSFeedEmpty = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
  <title>WFMU's recent playlists from Empty Show</title>
  <link>http://www.wfmu.org/playlists/ES</link>
  <description>No episodes</description>
  <language>en</language>
</channel>
</rss>`

// =============================================================================
// DJ Index Parsing Tests
// =============================================================================

func TestWFMU_ParseDJIndex_DiscoverShows(t *testing.T) {
	shows, err := parseWFMUDJIndex([]byte(wfmuDJIndexHTML))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(shows), 4, "should discover at least 4 shows")

	// Build a map for easier assertions
	showMap := make(map[string]RadioShowImport)
	for _, s := range shows {
		showMap[s.ExternalID] = s
	}

	// Check Wake
	wake, ok := showMap["WA"]
	assert.True(t, ok, "should discover Wake (WA)")
	if ok {
		assert.Equal(t, "Wake", wake.Name)
		assert.NotNil(t, wake.HostName)
		assert.Equal(t, "Trouble", *wake.HostName)
		assert.NotNil(t, wake.ArchiveURL)
		assert.Contains(t, *wake.ArchiveURL, "/playlists/WA")
	}

	// Check Surface Noise
	sn, ok := showMap["SN"]
	assert.True(t, ok, "should discover Surface Noise (SN)")
	if ok {
		assert.Equal(t, "Surface Noise", sn.Name)
		assert.NotNil(t, sn.HostName)
		assert.Equal(t, "Dave the Spade", *sn.HostName)
	}

	// Check Nervous Boogie
	nb, ok := showMap["NB"]
	assert.True(t, ok, "should discover Nervous Boogie (NB)")
	if ok {
		assert.Equal(t, "Nervous Boogie", nb.Name)
		assert.NotNil(t, nb.HostName)
		assert.Equal(t, "Richard J.", *nb.HostName)
	}

	// Check Shmelting (in "The Rest" section)
	s5, ok := showMap["S5"]
	assert.True(t, ok, "should discover Shmelting (S5)")
	if ok {
		assert.Equal(t, "Shmelting", s5.Name)
		assert.NotNil(t, s5.HostName)
		assert.Equal(t, "Zabedon", *s5.HostName)
	}
}

func TestWFMU_ParseDJIndex_Deduplication(t *testing.T) {
	// Create HTML with duplicate show codes (same show listed in multiple sections)
	duplicateHTML := `<html><body>
<p><b>Wake</b> with Trouble - <a href="/playlists/WA">playlists</a></p>
<p><b>Wake</b> with Trouble - <a href="/playlists/WA">archives</a></p>
</body></html>`

	shows, err := parseWFMUDJIndex([]byte(duplicateHTML))
	require.NoError(t, err)

	codeCount := 0
	for _, s := range shows {
		if s.ExternalID == "WA" {
			codeCount++
		}
	}
	assert.Equal(t, 1, codeCount, "should deduplicate shows by code")
}

func TestWFMU_ExtractShowCode(t *testing.T) {
	tests := []struct {
		href     string
		expected string
	}{
		{"/playlists/WA", "WA"},
		{"/playlists/SN", "SN"},
		{"/playlists/S5", "S5"},
		{"/playlists/NB", "NB"},
		{"/playlists/shows/12345", ""},  // episode link, not show
		{"/playlists/", ""},             // index page
		{"/playlists/shows", ""},        // filtered out
		{"/playlists/search", ""},       // filtered out
		{"/playlists/index", ""},        // filtered out
		{"/archivefeed/mp3/WA.xml", ""}, // feed link
		{"", ""},                        // empty
	}

	for _, tc := range tests {
		t.Run(tc.href, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractShowCode(tc.href))
		})
	}
}

func TestWFMU_ExtractDJName(t *testing.T) {
	tests := []struct {
		text     string
		showName string
		expected string
	}{
		{"Wake with Trouble playlists and archives", "Wake", "Trouble"},
		{"Surface Noise with Dave the Spade - playlists", "Surface Noise", "Dave the Spade"},
		{"Nervous Boogie with Richard J.", "Nervous Boogie", "Richard J."},
		{"Show Without Host", "Show Without Host", ""},
	}

	for _, tc := range tests {
		t.Run(tc.showName, func(t *testing.T) {
			result := extractDJName(tc.text, tc.showName)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// =============================================================================
// RSS Feed Parsing Tests
// =============================================================================

func TestWFMU_ParseRSSFeed_AllEpisodes(t *testing.T) {
	// Use a "since" time that's before all items
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	episodes, err := parseWFMURSSFeed([]byte(wfmuRSSFeed), "NB", since, time.Time{})
	require.NoError(t, err)
	assert.Len(t, episodes, 4, "should parse all 4 episodes")

	// Check first episode
	ep := episodes[0]
	assert.Equal(t, "162145", ep.ExternalID)
	assert.Equal(t, "NB", ep.ShowExternalID)
	assert.Equal(t, "2026-03-12", ep.AirDate)
	assert.NotNil(t, ep.Title)
	assert.Contains(t, *ep.Title, "Nervous Boogie")
	assert.NotNil(t, ep.ArchiveURL)
	assert.Contains(t, *ep.ArchiveURL, "/playlists/shows/162145")
}

func TestWFMU_ParseRSSFeed_SinceFilter(t *testing.T) {
	// Only get episodes since March 1, 2026
	since := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	episodes, err := parseWFMURSSFeed([]byte(wfmuRSSFeed), "NB", since, time.Time{})
	require.NoError(t, err)
	assert.Len(t, episodes, 2, "should only return episodes after March 1")

	// Both should be March episodes
	for _, ep := range episodes {
		assert.True(t, strings.HasPrefix(ep.AirDate, "2026-03"),
			"expected March 2026 episode, got %s", ep.AirDate)
	}
}

func TestWFMU_ParseRSSFeed_Empty(t *testing.T) {
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	episodes, err := parseWFMURSSFeed([]byte(wfmuRSSFeedEmpty), "ES", since, time.Time{})
	require.NoError(t, err)
	assert.Empty(t, episodes, "should return empty for feed with no items")
}

func TestWFMU_ParseRSSFeed_MalformedXML(t *testing.T) {
	_, err := parseWFMURSSFeed([]byte("this is not xml"), "XX", time.Time{}, time.Time{})
	assert.Error(t, err, "should error on malformed XML")
}

func TestWFMU_ExtractEpisodeID(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"http://www.wfmu.org/playlists/shows/162145", "162145"},
		{"https://wfmu.org/playlists/shows/161980", "161980"},
		{"/playlists/shows/123456", "123456"},
		{"http://www.wfmu.org/playlists/NB", ""},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractEpisodeID(tc.url))
		})
	}
}

// =============================================================================
// Playlist Page Parsing Tests
// =============================================================================

func TestWFMU_ParsePlaylistPage_AllFields(t *testing.T) {
	plays, err := parseWFMUPlaylistPage([]byte(wfmuPlaylistPageHTML))
	require.NoError(t, err)
	assert.Len(t, plays, 7, "should parse all 7 tracks")

	// Check first track — minimal fields
	p0 := plays[0]
	assert.Equal(t, 0, p0.Position)
	assert.Equal(t, "Sir Lattimore Brown", p0.ArtistName)
	require.NotNil(t, p0.TrackTitle)
	assert.Equal(t, "Shake and Vibrate", *p0.TrackTitle)
	require.NotNil(t, p0.LabelName)
	assert.Equal(t, "Sound Stage 7", *p0.LabelName)
	assert.Nil(t, p0.AlbumTitle, "no album for single")
	assert.Nil(t, p0.ReleaseYear, "no year specified")
	assert.False(t, p0.IsNew)

	// Check V.I.P.'s — has DJ comment
	p1 := plays[1]
	assert.Equal(t, "The V.I.P.'s", p1.ArtistName)
	require.NotNil(t, p1.DJComment)
	assert.Equal(t, "Orlons sound!", *p1.DJComment)

	// Check Ramona King — has year
	p2 := plays[2]
	assert.Equal(t, "Ramona King", p2.ArtistName)
	require.NotNil(t, p2.ReleaseYear)
	assert.Equal(t, 1964, *p2.ReleaseYear)

	// Check The Front Page — has album, label, year, comment, and is marked New
	p3 := plays[3]
	assert.Equal(t, "The Front Page & Her", p3.ArtistName)
	require.NotNil(t, p3.AlbumTitle)
	assert.Equal(t, "Hearts For Sale! Girl Group Sounds USA 1961-1967", *p3.AlbumTitle)
	require.NotNil(t, p3.LabelName)
	assert.Equal(t, "Ace Records Ltd.", *p3.LabelName)
	require.NotNil(t, p3.ReleaseYear)
	assert.Equal(t, 2022, *p3.ReleaseYear)
	require.NotNil(t, p3.DJComment)
	assert.Equal(t, "Fantastic comp of girl group rarities", *p3.DJComment)
	assert.True(t, p3.IsNew, "Front Page should be flagged as new")
}

func TestWFMU_ParsePlaylistPage_DJComments(t *testing.T) {
	plays, err := parseWFMUPlaylistPage([]byte(wfmuPlaylistPageHTML))
	require.NoError(t, err)

	// Collect all comments
	var comments []string
	for _, p := range plays {
		if p.DJComment != nil {
			comments = append(comments, *p.DJComment)
		}
	}

	assert.GreaterOrEqual(t, len(comments), 3, "should have at least 3 DJ comments")

	// Verify specific comments are preserved as-is
	commentSet := make(map[string]bool)
	for _, c := range comments {
		commentSet[c] = true
	}
	assert.True(t, commentSet["Orlons sound!"], "should preserve 'Orlons sound!' comment")
	assert.True(t, commentSet["Stone cold classic"], "should preserve 'Stone cold classic' comment")
	assert.True(t, commentSet["Amazing reissue from Numero. DJ Pick of the week!"],
		"should preserve multi-sentence DJ comment")
}

func TestWFMU_ParsePlaylistPage_NewReleaseFlag(t *testing.T) {
	plays, err := parseWFMUPlaylistPage([]byte(wfmuPlaylistPageHTML))
	require.NoError(t, err)

	newCount := 0
	for _, p := range plays {
		if p.IsNew {
			newCount++
		}
	}

	assert.Equal(t, 2, newCount, "should have 2 tracks flagged as new")

	// Check specific tracks
	assert.True(t, plays[3].IsNew, "Front Page should be new (text 'New')")
	assert.True(t, plays[6].IsNew, "Jody Reynolds should be new (image with alt 'new release')")
}

func TestWFMU_ParsePlaylistPage_PledgePromoFiltering(t *testing.T) {
	plays, err := parseWFMUPlaylistPage([]byte(wfmuPlaylistWithPromosHTML))
	require.NoError(t, err)

	// Should only have real music tracks (The Sonics, Link Wray, The Cramps)
	assert.Len(t, plays, 3, "should filter out pledge promos and station IDs")

	// Verify the remaining tracks
	assert.Equal(t, "The Sonics", plays[0].ArtistName)
	assert.Equal(t, "Link Wray", plays[1].ArtistName)
	assert.Equal(t, "The Cramps", plays[2].ArtistName)

	// Verify positions are re-numbered sequentially
	assert.Equal(t, 0, plays[0].Position)
	assert.Equal(t, 1, plays[1].Position)
	assert.Equal(t, 2, plays[2].Position)
}

func TestWFMU_ParsePlaylistPage_Empty(t *testing.T) {
	plays, err := parseWFMUPlaylistPage([]byte(wfmuEmptyPlaylistHTML))
	require.NoError(t, err)
	assert.Empty(t, plays, "should return empty for playlist with no tracks")
}

func TestWFMU_ParsePlaylistPage_MinimalColumns(t *testing.T) {
	plays, err := parseWFMUPlaylistPage([]byte(wfmuMinimalColumnsHTML))
	require.NoError(t, err)
	assert.Len(t, plays, 2, "should parse tracks with minimal columns")

	assert.Equal(t, "The Stooges", plays[0].ArtistName)
	require.NotNil(t, plays[0].TrackTitle)
	assert.Equal(t, "I Wanna Be Your Dog", *plays[0].TrackTitle)
	require.NotNil(t, plays[0].AlbumTitle)
	assert.Equal(t, "The Stooges", *plays[0].AlbumTitle)

	assert.Equal(t, "MC5", plays[1].ArtistName)
	require.NotNil(t, plays[1].TrackTitle)
	assert.Equal(t, "Kick Out the Jams", *plays[1].TrackTitle)
}

func TestWFMU_ParsePlaylistPage_MalformedHTML(t *testing.T) {
	// HTML with no table
	plays, err := parseWFMUPlaylistPage([]byte(`<html><body><p>No playlist here</p></body></html>`))
	require.NoError(t, err)
	assert.Empty(t, plays, "should return empty for HTML with no playlist table")
}

// =============================================================================
// Pledge/Promo Filtering Tests
// =============================================================================

func TestWFMU_IsPledgeOrPromo(t *testing.T) {
	tests := []struct {
		name     string
		row      wfmuPlaylistRow
		expected bool
	}{
		{
			name:     "normal track",
			row:      wfmuPlaylistRow{Artist: "The Sonics", Track: "Psycho"},
			expected: false,
		},
		{
			name:     "WFMU station",
			row:      wfmuPlaylistRow{Artist: "WFMU", Track: "Station Break"},
			expected: true,
		},
		{
			name:     "station ID",
			row:      wfmuPlaylistRow{Artist: "Station ID", Track: "WFMU 91.1"},
			expected: true,
		},
		{
			name:     "pledge in track",
			row:      wfmuPlaylistRow{Artist: "Various", Track: "Pledge Drive Promo"},
			expected: true,
		},
		{
			name:     "fundraiser in comments",
			row:      wfmuPlaylistRow{Artist: "Various", Track: "Spot", Comments: "Fundraiser announcement"},
			expected: true,
		},
		{
			name:     "donate in track",
			row:      wfmuPlaylistRow{Artist: "Announcer", Track: "Please donate now!"},
			expected: true,
		},
		{
			name:     "station break",
			row:      wfmuPlaylistRow{Artist: "Station Break", Track: ""},
			expected: true,
		},
		{
			name:     "PSA",
			row:      wfmuPlaylistRow{Artist: "Various", Track: "PSA: Community Event"},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isPledgeOrPromo(tc.row))
		})
	}
}

// =============================================================================
// Integration Tests (with httptest server)
// =============================================================================

func TestWFMU_DiscoverShows_Integration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlists/":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, wfmuDJIndexHTML)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	shows, err := provider.DiscoverShows()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(shows), 4)

	// Verify all shows have required fields
	for _, s := range shows {
		assert.NotEmpty(t, s.ExternalID, "show should have external ID")
		assert.NotEmpty(t, s.Name, "show should have name")
	}
}

func TestWFMU_FetchNewEpisodes_Integration(t *testing.T) {
	// Track which path was taken — recent `since` should hit the RSS feed,
	// NOT the archive page.
	var rssHits, archiveHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlistfeed/NB.xml":
			rssHits++
			w.Header().Set("Content-Type", "application/rss+xml")
			fmt.Fprint(w, wfmuRSSFeed)
		case "/playlists/NB":
			archiveHits++
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	// Recent `since` — should use RSS path.
	since := time.Now().Add(-7 * 24 * time.Hour)
	episodes, err := provider.FetchNewEpisodes("NB", since, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, 1, rssHits, "recent since should hit the RSS feed")
	assert.Equal(t, 0, archiveHits, "recent since should NOT hit the archive page")

	// Episodes returned depend on how many RSS items are newer than `since`;
	// the contract we verify is that every returned episode has the expected
	// shape.
	for _, ep := range episodes {
		assert.NotEmpty(t, ep.ExternalID)
		assert.Equal(t, "NB", ep.ShowExternalID)
		assert.NotEmpty(t, ep.AirDate)
	}
}

func TestWFMU_FetchPlaylist_Integration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlists/shows/162145":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, wfmuPlaylistPageHTML)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	plays, err := provider.FetchPlaylist("162145")
	require.NoError(t, err)
	assert.Len(t, plays, 7)

	// No MusicBrainz IDs for WFMU tracks
	for _, p := range plays {
		assert.Nil(t, p.MusicBrainzArtistID, "WFMU tracks should not have MB artist IDs")
		assert.Nil(t, p.MusicBrainzRecordingID, "WFMU tracks should not have MB recording IDs")
		assert.Nil(t, p.MusicBrainzReleaseID, "WFMU tracks should not have MB release IDs")
	}
}

// =============================================================================
// Error Handling Tests
// =============================================================================

func TestWFMU_HTTPError_DiscoverShows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Internal Server Error")
	}))
	defer server.Close()

	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	_, err := provider.DiscoverShows()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestWFMU_HTTPError_FetchNewEpisodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Not Found")
	}))
	defer server.Close()

	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	_, err := provider.FetchNewEpisodes("INVALID", time.Now(), time.Time{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestWFMU_HTTPError_FetchPlaylist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "Service Unavailable")
	}))
	defer server.Close()

	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	_, err := provider.FetchPlaylist("999999")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

// =============================================================================
// Rate Limiting Test
// =============================================================================

func TestWFMU_RateLimiting(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch {
		case strings.HasSuffix(r.URL.Path, "/playlists/"):
			fmt.Fprint(w, wfmuDJIndexHTML)
		case strings.HasSuffix(r.URL.Path, ".xml"):
			fmt.Fprint(w, wfmuRSSFeedEmpty)
		default:
			fmt.Fprint(w, wfmuEmptyPlaylistHTML)
		}
	}))
	defer server.Close()

	// Create provider with known rate limit
	provider := &WFMUProvider{
		httpClient:  server.Client(),
		baseURL:     server.URL,
		rateLimiter: time.NewTicker(50 * time.Millisecond),
	}
	defer provider.Close()

	// Make 3 requests and measure time
	start := time.Now()

	_, _ = provider.DiscoverShows()
	_, _ = provider.FetchNewEpisodes("WA", time.Now(), time.Time{})
	_, _ = provider.FetchPlaylist("12345")

	elapsed := time.Since(start)

	// With 50ms rate limit and 3 requests, should take at least 100ms (2 waits between 3 requests)
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(80),
		"3 requests with 50ms rate limit should take at least ~100ms, took %v", elapsed)
}

// =============================================================================
// Provider Close Test
// =============================================================================

func TestWFMU_Close(t *testing.T) {
	provider := NewWFMUProvider()
	assert.NotPanics(t, func() {
		provider.Close()
	})

	// Close again should not panic
	assert.NotPanics(t, func() {
		provider.Close()
	})
}

// =============================================================================
// User-Agent Test
// =============================================================================

func TestWFMU_UserAgent(t *testing.T) {
	var capturedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		fmt.Fprint(w, wfmuEmptyPlaylistHTML)
	}))
	defer server.Close()

	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	_, _ = provider.FetchPlaylist("123")
	assert.Equal(t, wfmuUserAgent, capturedUA, "should send correct User-Agent header")
}

// =============================================================================
// Helper function tests
// =============================================================================

func TestWFMU_CleanDJName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"DJ Shadow", "DJ Shadow"},
		{"DJ Shadow playlists and archives", "DJ Shadow"},
		{"DJ Shadow - RSS feeds", "DJ Shadow"},
		{"Richard J.", "Richard J."},
		{"Trouble playlists", "Trouble"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, cleanDJName(tc.input))
		})
	}
}

func TestWFMU_ParseWFMUReleaseYear(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"2023", 2023},
		{"1965", 1965},
		{"Released in 2022", 2022},
		{"abc", 0},
		{"", 0},
		{"123", 0},
		{"0000", 0},
		{"9999", 0},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, parseWFMUReleaseYear(tc.input))
		})
	}
}

func TestWFMU_ExtractDateFromTitle(t *testing.T) {
	tests := []struct {
		title    string
		expected string
	}{
		{"WFMU Playlist: Nervous Boogie from March 12, 2026", "2026-03-12"},
		{"WFMU Playlist: Wake from January 5, 2026", "2026-01-05"},
		{"WFMU Playlist: Show from December 31, 2025", "2025-12-31"},
		{"Some random title without a date", ""},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractDateFromTitle(tc.title))
		})
	}
}

// =============================================================================
// Archive Page Parsing Tests — Historical Backfill (PSY-278)
// =============================================================================

// loadWFMUTestdata reads a fixture file from ./testdata and returns its bytes.
func loadWFMUTestdata(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading testdata file %s", path)
	return data
}

func TestWFMU_ParseArchivePage_AllEpisodes(t *testing.T) {
	body := loadWFMUTestdata(t, "wfmu_archive_page_bt.html")

	episodes, err := parseWFMUArchivePage(body, "BT", time.Time{}, time.Time{})
	require.NoError(t, err)

	// The fixture has 7 <li> rows total:
	//   - 5 modern /playlists/shows/{ID} episodes
	//   - 1 legacy pre-2009 episode (skipped — no /playlists/shows/ link)
	//   - 2 fill-in placeholders (skipped — no playlist link at all)
	assert.Len(t, episodes, 5, "should parse 5 modern episodes, skip fill-ins and legacy")

	// Build a map for easier assertions
	byID := make(map[string]RadioEpisodeImport)
	for _, ep := range episodes {
		byID[ep.ExternalID] = ep
	}

	// Verify each expected episode is present with correct date
	expected := map[string]string{
		"78951": "2018-05-08",
		"78776": "2018-05-01",
		"78585": "2018-04-24",
		"78458": "2018-04-17",
		"77868": "2018-03-13",
	}
	for id, date := range expected {
		ep, ok := byID[id]
		assert.True(t, ok, "episode %s should be present", id)
		if ok {
			assert.Equal(t, date, ep.AirDate, "episode %s air date", id)
			assert.Equal(t, "BT", ep.ShowExternalID)
			assert.NotNil(t, ep.ArchiveURL, "episode %s should have archive URL", id)
			if ep.ArchiveURL != nil {
				assert.Contains(t, *ep.ArchiveURL, "/playlists/shows/"+id)
			}
		}
	}

	// Legacy pre-2009 episode must NOT be present
	_, hasLegacy := byID["135"]
	assert.False(t, hasLegacy, "pre-2009 legacy episode 135 should be skipped")

	// Fill-in placeholders must NOT be present
	_, hasFillIn1 := byID["78094"]
	assert.False(t, hasFillIn1, "fill-in placeholder 78094 should be skipped")
	_, hasFillIn2 := byID["77975"]
	assert.False(t, hasFillIn2, "fill-in placeholder 77975 should be skipped")
}

func TestWFMU_ParseArchivePage_Titles(t *testing.T) {
	body := loadWFMUTestdata(t, "wfmu_archive_page_bt.html")

	episodes, err := parseWFMUArchivePage(body, "BT", time.Time{}, time.Time{})
	require.NoError(t, err)

	byID := make(map[string]RadioEpisodeImport)
	for _, ep := range episodes {
		byID[ep.ExternalID] = ep
	}

	// Episode with a non-empty <b> title
	ep78776, ok := byID["78776"]
	require.True(t, ok)
	require.NotNil(t, ep78776.Title, "episode 78776 should have a title")
	assert.Equal(t, "Retrospective on the show's assorted live sessions", *ep78776.Title)

	ep77868, ok := byID["77868"]
	require.True(t, ok)
	require.NotNil(t, ep77868.Title, "episode 77868 should have a title")
	assert.Equal(t, "Marathon Week 2 w/ co-host Fabio", *ep77868.Title)

	// Episodes with empty <b> titles should have no Title set
	ep78951, ok := byID["78951"]
	require.True(t, ok)
	assert.Nil(t, ep78951.Title, "episode 78951 has empty <b> — title should be nil")
}

func TestWFMU_ParseArchivePage_SinceFilter(t *testing.T) {
	body := loadWFMUTestdata(t, "wfmu_archive_page_bt.html")

	// Filter to episodes on or after April 20, 2018
	since := time.Date(2018, 4, 20, 0, 0, 0, 0, time.UTC)
	episodes, err := parseWFMUArchivePage(body, "BT", since, time.Time{})
	require.NoError(t, err)

	// Should return only episodes from April 24, May 1, May 8 (3 episodes)
	assert.Len(t, episodes, 3, "should filter episodes older than since")

	for _, ep := range episodes {
		airTime, err := time.Parse("2006-01-02", ep.AirDate)
		require.NoError(t, err)
		assert.False(t, airTime.Before(since),
			"episode %s (%s) should not be before since", ep.ExternalID, ep.AirDate)
	}
}

func TestWFMU_ParseArchivePage_SinceFilter_AllOld(t *testing.T) {
	body := loadWFMUTestdata(t, "wfmu_archive_page_bt.html")

	// Filter to future — should return nothing
	since := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	episodes, err := parseWFMUArchivePage(body, "BT", since, time.Time{})
	require.NoError(t, err)
	assert.Empty(t, episodes, "no episodes should match a future since")
}

func TestWFMU_ParseArchivePage_Empty(t *testing.T) {
	body := loadWFMUTestdata(t, "wfmu_archive_page_empty.html")

	episodes, err := parseWFMUArchivePage(body, "EMPTY", time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Empty(t, episodes, "empty archive should return empty slice")
}

func TestWFMU_ParseArchivePage_NoShowlist(t *testing.T) {
	// HTML with no <div class="showlist"> — should return empty, not error.
	body := []byte(`<!DOCTYPE html><html><body><p>Nothing here.</p></body></html>`)

	episodes, err := parseWFMUArchivePage(body, "XX", time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Empty(t, episodes, "missing showlist should return empty")
}

func TestWFMU_ParseArchivePage_MalformedRows(t *testing.T) {
	// Showlist present but rows have missing/malformed data.
	body := []byte(`<!DOCTYPE html><html><body>
<div class="showlist">
<ul>
<li>
  <span class="KDBFavIcon KDBepisode" id="KDBepisode-100"></span>
  January 10, 2024:
  <a href="/playlists/shows/100">See the playlist</a>
</li>
<li>
  <!-- No KDBepisode span, no date, no link -->
  Just some text
</li>
<li>
  <!-- Has link but no date -->
  <span class="KDBFavIcon KDBepisode" id="KDBepisode-101"></span>
  <a href="/playlists/shows/101">See the playlist</a>
</li>
<li>
  <!-- Date but no link (fill-in placeholder) -->
  <span class="KDBFavIcon KDBepisode" id="KDBepisode-102"></span>
  January 17, 2024:
  <b>Guest host filled in.</b>
</li>
<li>
  <!-- Valid episode -->
  <span class="KDBFavIcon KDBepisode" id="KDBepisode-103"></span>
  January 24, 2024:
  <b>Good one</b>
  <a href="/playlists/shows/103">See the playlist</a>
</li>
</ul>
</div>
</body></html>`)

	episodes, err := parseWFMUArchivePage(body, "XX", time.Time{}, time.Time{})
	require.NoError(t, err)

	// Should parse: 100 (valid), 103 (valid). Skip: 101 (no date), 102 (no link), row 2 (nothing).
	assert.Len(t, episodes, 2, "should gracefully skip malformed rows")

	byID := make(map[string]RadioEpisodeImport)
	for _, ep := range episodes {
		byID[ep.ExternalID] = ep
	}
	assert.Contains(t, byID, "100")
	assert.Contains(t, byID, "103")
	assert.Equal(t, "2024-01-10", byID["100"].AirDate)
	assert.Equal(t, "2024-01-24", byID["103"].AirDate)
}

func TestWFMU_ParseArchivePage_Deduplication(t *testing.T) {
	// Duplicate episode IDs should be deduped.
	body := []byte(`<!DOCTYPE html><html><body>
<div class="showlist">
<ul>
<li>
  <span class="KDBFavIcon KDBepisode" id="KDBepisode-500"></span>
  March 1, 2024:
  <a href="/playlists/shows/500">See the playlist</a>
</li>
<li>
  <span class="KDBFavIcon KDBepisode" id="KDBepisode-500"></span>
  March 1, 2024:
  <a href="/playlists/shows/500">See the playlist</a>
</li>
</ul>
</div>
</body></html>`)

	episodes, err := parseWFMUArchivePage(body, "XX", time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Len(t, episodes, 1, "duplicate episode IDs should be deduplicated")
}

func TestWFMU_ExtractArchiveDate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"may", "May 8, 2018: episode", "2018-05-08"},
		{"january", "January 2, 2001:", "2001-01-02"},
		{"december", "December 31, 2025: something", "2025-12-31"},
		{"october two digits", "October 15, 2019:", "2019-10-15"},
		{"lowercase", "march 12, 2026:", "2026-03-12"},
		{"surrounded by text", "...May 8, 2018: More text", "2018-05-08"},
		{"no date", "some text without any date", ""},
		{"empty", "", ""},
		{"bad day", "May 99, 2018:", ""},
		{"bad year", "May 8, 1500:", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractArchiveDate(tc.input))
		})
	}
}

// =============================================================================
// Archive Page Integration Tests
// =============================================================================

func TestWFMU_FetchNewEpisodes_ArchiveFallback_Integration(t *testing.T) {
	archiveBody := loadWFMUTestdata(t, "wfmu_archive_page_bt.html")

	var rssHits, archiveHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlistfeed/BT.xml":
			rssHits++
			w.Header().Set("Content-Type", "application/rss+xml")
			fmt.Fprint(w, wfmuRSSFeed)
		case "/playlists/BT":
			archiveHits++
			w.Header().Set("Content-Type", "text/html")
			w.Write(archiveBody)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	// Old `since` (beyond the 14-day window) triggers the archive page path.
	since := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)
	episodes, err := provider.FetchNewEpisodes("BT", since, time.Time{})
	require.NoError(t, err)

	assert.Equal(t, 0, rssHits, "historical since should NOT hit RSS")
	assert.Equal(t, 1, archiveHits, "historical since should hit archive page")
	assert.Len(t, episodes, 5, "should return all 5 modern episodes from fixture")

	for _, ep := range episodes {
		assert.Equal(t, "BT", ep.ShowExternalID)
		assert.NotEmpty(t, ep.ExternalID)
		assert.NotEmpty(t, ep.AirDate)
	}
}

func TestWFMU_FetchNewEpisodes_ArchiveFallback_ZeroSince(t *testing.T) {
	archiveBody := loadWFMUTestdata(t, "wfmu_archive_page_bt.html")

	var archiveHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/playlists/BT" {
			archiveHits++
			w.Write(archiveBody)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	// Zero `since` means "all history" → archive page.
	episodes, err := provider.FetchNewEpisodes("BT", time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, 1, archiveHits)
	assert.Len(t, episodes, 5)
}

func TestWFMU_FetchNewEpisodes_ArchiveFallback_404(t *testing.T) {
	// Unknown show codes should return empty, not error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Not Found")
	}))
	defer server.Close()

	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	// Old `since` → archive path; 404 → empty slice, no error.
	episodes, err := provider.FetchNewEpisodes("NOPE", time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	require.NoError(t, err, "404 from archive page should become empty slice")
	assert.Empty(t, episodes)
}
