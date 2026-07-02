package pipeline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Browse-by-RG paginates to completion and decodes status/date/url-rels
// (including the ended flag) for each release.
func TestMusicBrainzClient_BrowseReleaseURLRelations(t *testing.T) {
	var offsets []string
	mux := http.NewServeMux()
	mux.HandleFunc("/release", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "rg-mbid", r.URL.Query().Get("release-group"))
		assert.Equal(t, "url-rels", r.URL.Query().Get("inc"))
		assert.Equal(t, "json", r.URL.Query().Get("fmt"))
		assert.Equal(t, "100", r.URL.Query().Get("limit"))
		off := r.URL.Query().Get("offset")
		offsets = append(offsets, off)
		w.Header().Set("Content-Type", "application/json")
		switch off {
		case "0":
			_, _ = w.Write([]byte(`{"release-count":2,"releases":[
				{"id":"rel-1","title":"Punisher","status":"Official","date":"2020-06-18","relations":[
					{"type":"free streaming","ended":false,"url":{"resource":"https://phoebe.bandcamp.com/album/punisher"}},
					{"type":"streaming","ended":true,"url":{"resource":"https://open.spotify.com/album/dead"}}
				]}
			]}`))
		default:
			_, _ = w.Write([]byte(`{"release-count":2,"releases":[
				{"id":"rel-2","title":"Punisher (promo)","status":"Promotion","date":"2020","relations":[]}
			]}`))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newReleaseGroupTestClient(srv.URL)
	rels, err := c.BrowseReleaseURLRelations(context.Background(), "rg-mbid")
	require.NoError(t, err)

	require.Len(t, rels, 2, "both pages walked")
	assert.Equal(t, "rel-1", rels[0].ID)
	assert.Equal(t, "Official", rels[0].Status)
	require.Len(t, rels[0].Relations, 2)
	assert.Equal(t, "https://phoebe.bandcamp.com/album/punisher", rels[0].Relations[0].URL.Resource)
	assert.False(t, rels[0].Relations[0].Ended)
	assert.True(t, rels[0].Relations[1].Ended, "ended flag decoded")
	assert.Equal(t, "Promotion", rels[1].Status)
	assert.Equal(t, []string{"0", "1"}, offsets, "paginated by page length until count reached")
}

// Empty MBID short-circuits without any HTTP call.
func TestMusicBrainzClient_BrowseReleaseURLRelations_EmptyMBIDNoCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("no HTTP call expected for an empty MBID")
	}))
	defer srv.Close()

	c := newReleaseGroupTestClient(srv.URL)
	rels, err := c.BrowseReleaseURLRelations(context.Background(), "  ")
	require.NoError(t, err)
	assert.Nil(t, rels)
}

// The release-flavored classifier accepts album/track pages on both platforms,
// rejects artist/profile/playlist URLs, and canonicalizes.
func TestClassifyReleasePlatformURL(t *testing.T) {
	cases := []struct {
		name         string
		in           string
		wantPlatform string
		wantURL      string
		wantOK       bool
	}{
		{"bandcamp album", "https://phoebe.bandcamp.com/album/punisher/", "bandcamp", "https://phoebe.bandcamp.com/album/punisher", true},
		{"bandcamp track", "http://Artist.Bandcamp.com/track/kyoto?from=search", "bandcamp", "https://artist.bandcamp.com/track/kyoto", true},
		{"bandcamp bare profile rejected", "https://phoebe.bandcamp.com", "", "", false},
		{"spotify album", "https://open.spotify.com/album/6Pp6qGEywDdofgFC1oFbSH", "spotify", "https://open.spotify.com/album/6Pp6qGEywDdofgFC1oFbSH", true},
		{"spotify artist rejected", "https://open.spotify.com/artist/1r1uxoy19fzMxunt3ONAkG", "", "", false},
		{"spotify playlist rejected", "https://open.spotify.com/playlist/xyz", "", "", false},
		{"non-platform host rejected", "https://evil.example.com/album/punisher", "", "", false},
		{"bandcamp-in-path attack rejected", "https://evil.example.com/bandcamp.com/album/x", "", "", false},
		{"non-http scheme rejected", "ftp://phoebe.bandcamp.com/album/punisher", "", "", false},
		{"garbage rejected", "://not a url", "", "", false},
		{"nested album substring rejected", "https://open.spotify.com/playlist/p/album/hidden", "", "", false},
		{"dot-segment escape rejected", "https://open.spotify.com/album/../../evil", "", "", false},
		{"encoded dot-segment escape rejected", "https://open.spotify.com/album/%2E%2E/%2E%2E/evil", "", "", false},
		{"empty slug rejected", "https://x.bandcamp.com/album/", "", "", false},
		{"spotify intl prefix accepted", "https://open.spotify.com/intl-pt/album/6Pp6qGEywDdofgFC1oFbSH", "spotify", "https://open.spotify.com/intl-pt/album/6Pp6qGEywDdofgFC1oFbSH", true},
		{"bandcamp intl-like segment rejected", "https://x.bandcamp.com/intl-pt/album/y", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, u, ok := ClassifyReleasePlatformURL(tc.in)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantPlatform, p)
			assert.Equal(t, tc.wantURL, u)
		})
	}
}
