package pipeline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Browse paginates to completion (offset-driven) AND filters to the requested primary
// types client-side: a Single is dropped, Album + EP across two pages are kept.
func TestMusicBrainzClient_BrowseArtistReleaseGroups(t *testing.T) {
	var offsets []string
	mux := http.NewServeMux()
	mux.HandleFunc("/release-group", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "art-mbid", r.URL.Query().Get("artist"))
		assert.Equal(t, "json", r.URL.Query().Get("fmt"))
		assert.Equal(t, "100", r.URL.Query().Get("limit"))
		off := r.URL.Query().Get("offset")
		offsets = append(offsets, off)
		w.Header().Set("Content-Type", "application/json")
		switch off {
		case "0":
			_, _ = w.Write([]byte(`{"release-group-count":3,"release-groups":[
				{"id":"rg-album","title":"LP One","primary-type":"Album","first-release-date":"2001-01-01"},
				{"id":"rg-single","title":"A Single","primary-type":"Single","first-release-date":"2002"}
			]}`))
		default: // offset advanced by the page length (2)
			_, _ = w.Write([]byte(`{"release-group-count":3,"release-groups":[
				{"id":"rg-ep","title":"An EP","primary-type":"EP","first-release-date":"2003-06"}
			]}`))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newReleaseGroupTestClient(srv.URL)
	rgs, err := c.BrowseArtistReleaseGroups(context.Background(), "art-mbid",
		map[string]bool{"album": true, "ep": true})
	require.NoError(t, err)

	require.Len(t, rgs, 2, "Single dropped; Album + EP kept across two pages")
	assert.Equal(t, "rg-album", rgs[0].ID)
	assert.Equal(t, "Album", rgs[0].PrimaryType)
	assert.Equal(t, "rg-ep", rgs[1].ID)
	assert.Equal(t, "EP", rgs[1].PrimaryType)
	assert.Equal(t, "2003-06", rgs[1].FirstReleaseDate)
	assert.Equal(t, []string{"0", "2"}, offsets, "paginated by page length until count reached")
}

// nil/empty primaryTypes keeps everything.
func TestMusicBrainzClient_BrowseArtistReleaseGroups_NoFilter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/release-group", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"release-group-count":2,"release-groups":[
			{"id":"a","primary-type":"Album"},{"id":"s","primary-type":"Single"}
		]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newReleaseGroupTestClient(srv.URL)
	rgs, err := c.BrowseArtistReleaseGroups(context.Background(), "art-mbid", nil)
	require.NoError(t, err)
	assert.Len(t, rgs, 2)
}

// An empty MBID makes no API call.
func TestMusicBrainzClient_BrowseArtistReleaseGroups_EmptyMBIDNoCall(t *testing.T) {
	var called bool
	mux := http.NewServeMux()
	mux.HandleFunc("/release-group", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"release-group-count":0,"release-groups":[]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newReleaseGroupTestClient(srv.URL)
	rgs, err := c.BrowseArtistReleaseGroups(context.Background(), "   ", map[string]bool{"album": true})
	require.NoError(t, err)
	assert.Nil(t, rgs)
	assert.False(t, called, "empty MBID must not hit the API")
}
