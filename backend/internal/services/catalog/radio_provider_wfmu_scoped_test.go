package catalog

// Tests for WFMU station-scoped show discovery (PSY-1073). The DJ index is
// one flat list spanning all four WFMU streams; these tests verify that
// DiscoverShowsForStation filters it down to per-stream catalogs using the
// /table + channel-landing-page rosters and the channel-artifact overrides.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Synthetic DJ index covering every ownership class:
//
//	WA — on the 91.1 /table schedule → flagship
//	CK — on /table AND the Rock'n'Soul roster (rebroadcast) → flagship
//	MY — on the Drummer roster only → drummer
//	SH — on the Sheena roster only → sheena
//	RQ — "Rock'n'Soul Radio" channel artifact (also on its roster) → rocknsoul
//	GW — "Give The Drummer Radio" channel artifact, on NO roster → drummer
//	JZ — "Sheena's Jungle Room Stream" channel artifact, on NO roster → sheena
//	ZZ — defunct, on no schedule page → flagship (unknown default)
const wfmuScopedDJIndexHTML = `<!DOCTYPE html>
<html><body>
<p><b>Wake</b> with Clay Pigeon <a href="/playlists/WA">playlists and archives</a></p>
<p><b>Cool Show</b> with CK Host <a href="/playlists/CK">playlists and archives</a></p>
<p><b>100% Whatever</b> with Mary Wing <a href="/playlists/MY">playlists and archives</a></p>
<p><b>Sheena Show</b> with Sheena Host <a href="/playlists/SH">playlists and archives</a></p>
<p><b>Rock'n'Soul Radio</b> <a href="/playlists/RQ">playlists and archives</a></p>
<p><b>Give The Drummer Radio</b> <a href="/playlists/GW">playlists and archives</a></p>
<p><b>Sheena's Jungle Room Stream</b> <a href="/playlists/JZ">playlists and archives</a></p>
<p><b>Defunct Show</b> with Old Host <a href="/playlists/ZZ">playlists and archives</a></p>
</body></html>`

const wfmuScopedTableHTML = `<!DOCTYPE html>
<html><body>
<a href="/playlists/WA" class="show-title-link">Wake</a>
<a href="/playlists/CK" class="show-title-link">Cool Show</a>
</body></html>`

// Drummer roster includes an absolute-URL cross-reference to WA inside a
// description — those must NOT count as roster membership (only exact
// relative /playlists/{CODE} hrefs do).
const wfmuScopedDrummerHTML = `<!DOCTYPE html>
<html><body>
<a href="/playlists/MY" title="Browse playlists &amp; archives">100% Whatever with Mary Wing</a>
<i>Also hear her on <a href="https://wfmu.org/playlists/WA">Wake</a> sometimes.</i>
</body></html>`

const wfmuScopedRockSoulHTML = `<!DOCTYPE html>
<html><body>
<a href="/playlists/RQ" title="Browse playlists &amp; archives">Rock'n'Soul Radio</a>
<a href="/playlists/CK" title="Browse playlists &amp; archives">Cool Show (91.1 rebroadcast)</a>
</body></html>`

const wfmuScopedSheenaHTML = `<!DOCTYPE html>
<html><body>
<a href="/playlists/SH" title="Browse playlists &amp; archives">Sheena Show</a>
</body></html>`

// newScopedWFMUTestServer serves the synthetic DJ index plus all four
// schedule pages.
func newScopedWFMUTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/playlists/":
			_, _ = fmt.Fprint(w, wfmuScopedDJIndexHTML)
		case "/table":
			_, _ = fmt.Fprint(w, wfmuScopedTableHTML)
		case "/drummer":
			_, _ = fmt.Fprint(w, wfmuScopedDrummerHTML)
		case "/rocknsoulradio":
			_, _ = fmt.Fprint(w, wfmuScopedRockSoulHTML)
		case "/sheena":
			_, _ = fmt.Fprint(w, wfmuScopedSheenaHTML)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func scopedShowCodes(shows []RadioShowImport) []string {
	codes := make([]string, 0, len(shows))
	for _, s := range shows {
		codes = append(codes, s.ExternalID)
	}
	return codes
}

func TestWFMU_DiscoverShowsForStation_ScopesByStream(t *testing.T) {
	server := newScopedWFMUTestServer(t)

	tests := []struct {
		stationSlug string
		wantCodes   []string
	}{
		// Flagship: 91.1 schedule (incl. the CK rebroadcast — /table wins)
		// plus the defunct unknown.
		{"wfmu", []string{"WA", "CK", "ZZ"}},
		// Drummer: roster show + the GW artifact pinned by override.
		{"wfmu-drummer", []string{"MY", "GW"}},
		// Rock'n'Soul: only its artifact — CK went to the flagship.
		{"wfmu-rocknsoulradio", []string{"RQ"}},
		// Sheena: roster show + the JZ artifact pinned by override.
		{"wfmu-sheena", []string{"SH", "JZ"}},
	}

	for _, tc := range tests {
		t.Run(tc.stationSlug, func(t *testing.T) {
			provider := NewWFMUProviderWithClient(server.Client(), server.URL)
			defer provider.Close()

			shows, err := provider.DiscoverShowsForStation(tc.stationSlug)
			require.NoError(t, err)
			assert.ElementsMatch(t, tc.wantCodes, scopedShowCodes(shows))
		})
	}
}

func TestWFMU_DiscoverShowsForStation_UnknownSlugErrors(t *testing.T) {
	server := newScopedWFMUTestServer(t)
	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	_, err := provider.DiscoverShowsForStation("wfmu-new-channel")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown WFMU station slug")
}

func TestWFMU_DiscoverShowsForStation_EmptyRosterErrors(t *testing.T) {
	// A schedule page that parses but contains no show links means the page
	// layout changed — discovery must fail loudly instead of silently
	// dumping that channel's shows on the flagship.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/playlists/":
			_, _ = fmt.Fprint(w, wfmuScopedDJIndexHTML)
		case "/table":
			_, _ = fmt.Fprint(w, wfmuScopedTableHTML)
		case "/drummer":
			_, _ = fmt.Fprint(w, `<html><body>maintenance page, no roster</body></html>`)
		case "/rocknsoulradio":
			_, _ = fmt.Fprint(w, wfmuScopedRockSoulHTML)
		case "/sheena":
			_, _ = fmt.Fprint(w, wfmuScopedSheenaHTML)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	_, err := provider.DiscoverShowsForStation("wfmu")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no show codes found")
}

func TestWFMU_FetchShowOwnership(t *testing.T) {
	server := newScopedWFMUTestServer(t)
	provider := NewWFMUProviderWithClient(server.Client(), server.URL)
	defer provider.Close()

	ownership, err := provider.FetchShowOwnership()
	require.NoError(t, err)

	assert.Equal(t, map[string]string{
		"WA": "wfmu",
		"CK": "wfmu", // /table beats the Rock'n'Soul rebroadcast listing
		"MY": "wfmu-drummer",
		"GW": "wfmu-drummer", // artifact override
		"RQ": "wfmu-rocknsoulradio",
		"SH": "wfmu-sheena",
		"JZ": "wfmu-sheena", // artifact override
		// ZZ intentionally absent: unknown codes default to the flagship
		// at the consumer (DiscoverShowsForStation / dedup).
	}, ownership)
}

func TestWFMU_FamilySlugs_MatchProviderChannelMap(t *testing.T) {
	// wfmuStationChannels (provider, discovery scoping) and WFMUFamilySlugs
	// (dedup command) describe the same station family. They are maintained
	// by hand in two files; this pins them together so adding a channel to
	// one without the other fails loudly instead of silently mis-scoping.
	assert.Len(t, wfmuStationChannels, len(WFMUFamilySlugs))
	for _, slug := range WFMUFamilySlugs {
		assert.Contains(t, wfmuStationChannels, slug)
	}
	assert.Contains(t, wfmuStationChannels, WFMUFlagshipSlug)
	assert.Equal(t, wfmuLiveChannelMain, wfmuStationChannels[WFMUFlagshipSlug])
}

func TestWFMU_ParseScheduleCodes_RealChannelLandingMarkup(t *testing.T) {
	// Mirrors the real channel-landing markup shape (KenzoDB expander rows)
	// to pin the relative-href extraction against the live page structure.
	page := `<html><body>
<span class="expander_section_with_desc">
<a href="/playlists/MY" title="Browse playlists &amp; archives">100% Whatever with Mary Wing</a>
<span id="expander_target_28">
<i>desc with cross-ref <a href="https://wfmu.org/playlists/GB">Someday Matinee</a></i>
</span></span>
<a href="/playlists/shows/162230">See the playlist</a>
<a href="/playlists/">index</a>
</body></html>`

	codes, err := parseWFMUScheduleCodes([]byte(page))
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"MY": true}, codes)
}
