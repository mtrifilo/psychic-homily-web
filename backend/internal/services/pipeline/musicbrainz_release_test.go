package pipeline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newReleaseGroupTestClient points a client at an httptest server with a fast
// throttle so the release-group search tests need no network and no ~1s wait.
func newReleaseGroupTestClient(baseURL string) *MusicBrainzClient {
	c := NewMusicBrainzClient()
	c.baseURL = baseURL
	c.rateLimit = time.Millisecond
	return c
}

func TestMusicBrainzClient_SearchReleaseGroups(t *testing.T) {
	var gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("/release-group/", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("query")
		assert.Equal(t, "json", r.URL.Query().Get("fmt"))
		assert.Equal(t, mbUserAgent, r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"release-groups":[
			{"id":"rg-1","title":"Dopesmoker","primary-type":"Album","first-release-date":"2003-02-04",
			 "artist-credit":[{"name":"Sleep","artist":{"id":"art-1","name":"Sleep"}}]}
		]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newReleaseGroupTestClient(srv.URL)
	rgs, err := c.SearchReleaseGroups(context.Background(), "Sleep", "Dopesmoker", 10)
	require.NoError(t, err)
	require.Len(t, rgs, 1)
	assert.Equal(t, "rg-1", rgs[0].ID)
	assert.Equal(t, "Dopesmoker", rgs[0].Title)
	assert.Equal(t, "2003-02-04", rgs[0].FirstReleaseDate)
	require.Len(t, rgs[0].ArtistCredit, 1)
	assert.Equal(t, "Sleep", rgs[0].ArtistCredit[0].Name)
	assert.Equal(t, "Sleep", rgs[0].ArtistCredit[0].Artist.Name)

	// Query embeds quoted artist + releasegroup phrases joined by AND.
	assert.Contains(t, gotQuery, `artist:"Sleep"`)
	assert.Contains(t, gotQuery, `releasegroup:"Dopesmoker"`)
	assert.Contains(t, gotQuery, " AND ")
}

func TestMusicBrainzClient_SearchReleaseGroups_EmptyArgsNoCall(t *testing.T) {
	var called bool
	mux := http.NewServeMux()
	mux.HandleFunc("/release-group/", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"release-groups":[]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newReleaseGroupTestClient(srv.URL)
	rgs, err := c.SearchReleaseGroups(context.Background(), "", "Dopesmoker", 10)
	require.NoError(t, err)
	assert.Nil(t, rgs)
	assert.False(t, called, "empty artist must not hit the API")
}

func TestMusicBrainzClient_SearchReleaseGroups_StripsInteriorQuotes(t *testing.T) {
	var gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("/release-group/", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("query")
		_, _ = w.Write([]byte(`{"release-groups":[]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newReleaseGroupTestClient(srv.URL)
	_, err := c.SearchReleaseGroups(context.Background(), `Sleep"`, `Dope"smoker`, 10)
	require.NoError(t, err)
	// Interior double quotes become spaces so a value can't break out of the phrase.
	assert.Contains(t, gotQuery, `artist:"Sleep "`)
	assert.Contains(t, gotQuery, `releasegroup:"Dope smoker"`)
}

func TestMusicBrainzClient_SearchReleaseGroups_RateLimited(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/release-group/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newReleaseGroupTestClient(srv.URL)
	_, err := c.SearchReleaseGroups(context.Background(), "Sleep", "Dopesmoker", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")
}
