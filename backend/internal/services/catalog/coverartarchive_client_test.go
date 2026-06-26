package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCAATestClient(t *testing.T, handler http.HandlerFunc) (*CoverArtArchiveClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewCoverArtArchiveClientWithConfig(srv.Client(), srv.URL, "https://musicbrainz.org")
	t.Cleanup(c.Close)
	return c, srv
}

func TestCAAClient_FrontCover_Found(t *testing.T) {
	c, srv := newCAATestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/release-group/mbid-1", r.URL.Path)
		assert.Equal(t, caaUserAgent, r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"images":[
			{"front":false,"image":"https://archive.org/back.jpg"},
			{"front":true,"image":"https://archive.org/front.jpg"}
		]}`))
	})

	res, err := c.FrontCover(context.Background(), "mbid-1")
	require.NoError(t, err)
	require.NotNil(t, res)
	// Stores the STABLE /front redirect endpoint, not the by-id direct image URL.
	assert.Equal(t, srv.URL+"/release-group/mbid-1/front", res.ImageURL)
	// Attribution linkback is the human MusicBrainz page.
	assert.Equal(t, "https://musicbrainz.org/release-group/mbid-1", res.SourceURL)
}

func TestCAAClient_FrontCover_NoFrontImage(t *testing.T) {
	c, _ := newCAATestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"images":[{"front":false,"image":"https://archive.org/back.jpg"}]}`))
	})

	res, err := c.FrontCover(context.Background(), "mbid-1")
	require.NoError(t, err)
	assert.Nil(t, res, "no image flagged front → no storable cover")
}

func TestCAAClient_FrontCover_NotFoundIsNotError(t *testing.T) {
	c, _ := newCAATestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	res, err := c.FrontCover(context.Background(), "mbid-1")
	require.NoError(t, err, "a 404 means no art for this release-group — a normal outcome")
	assert.Nil(t, res)
}

func TestCAAClient_FrontCover_ServerError(t *testing.T) {
	c, _ := newCAATestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.FrontCover(context.Background(), "mbid-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestCAAClient_FrontCover_EmptyMBIDNoCall(t *testing.T) {
	var called bool
	c, _ := newCAATestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})

	_, err := c.FrontCover(context.Background(), "  ")
	require.Error(t, err)
	assert.False(t, called, "empty mbid must not hit the API")
}
