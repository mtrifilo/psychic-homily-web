package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDiscogsTestClient(t *testing.T, token string, handler http.HandlerFunc) *DiscogsClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewDiscogsClientWithConfig(srv.Client(), srv.URL, "https://www.discogs.com", token)
	t.Cleanup(c.Close)
	return c
}

func TestDiscogsClient_SearchReleaseCovers(t *testing.T) {
	c := newDiscogsTestClient(t, "tok-123", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/database/search", r.URL.Path)
		assert.Equal(t, "Discogs token=tok-123", r.Header.Get("Authorization"))
		assert.Equal(t, discogsUserAgent, r.Header.Get("User-Agent"))
		q := r.URL.Query()
		assert.Equal(t, "release", q.Get("type"))
		assert.Equal(t, "Sleep", q.Get("artist"))
		assert.Equal(t, "Dopesmoker", q.Get("release_title"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":111,"type":"release","title":"Sleep - Dopesmoker","year":"2003",
			 "cover_image":"https://i.discogs.com/abc/front.jpeg"},
			{"id":222,"type":"master","title":"Sleep - Dopesmoker","year":"2003",
			 "cover_image":"https://i.discogs.com/def/front.jpeg"},
			{"id":333,"type":"release","title":"Sleep - Volume One","year":"1991",
			 "cover_image":"https://st.discogs.com/spacer.gif"}
		]}`))
	})

	rels, err := c.SearchReleaseCovers(context.Background(), "Sleep", "Dopesmoker", 10)
	require.NoError(t, err)
	// The master-type row and the spacer-image (non-i.discogs.com) row are dropped.
	require.Len(t, rels, 1)
	assert.Equal(t, int64(111), rels[0].ID)
	assert.Equal(t, "Sleep - Dopesmoker", rels[0].Title)
	assert.Equal(t, 2003, rels[0].Year)
	assert.Equal(t, "https://i.discogs.com/abc/front.jpeg", rels[0].CoverImage)
	assert.Equal(t, "https://www.discogs.com/release/111", rels[0].SourceURL)
}

func TestDiscogsClient_NoTokenOmitsAuthHeader(t *testing.T) {
	c := newDiscogsTestClient(t, "", func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"), "no token → no Authorization header")
		_, _ = w.Write([]byte(`{"results":[]}`))
	})

	_, err := c.SearchReleaseCovers(context.Background(), "Sleep", "Dopesmoker", 10)
	require.NoError(t, err)
}

func TestDiscogsClient_RateLimited(t *testing.T) {
	c := newDiscogsTestClient(t, "tok", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := c.SearchReleaseCovers(context.Background(), "Sleep", "Dopesmoker", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")
}

func TestDiscogsClient_ServerError(t *testing.T) {
	c := newDiscogsTestClient(t, "tok", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	})

	_, err := c.SearchReleaseCovers(context.Background(), "Sleep", "Dopesmoker", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestDiscogsClient_EmptyArgsNoCall(t *testing.T) {
	var called bool
	c := newDiscogsTestClient(t, "tok", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"results":[]}`))
	})

	rels, err := c.SearchReleaseCovers(context.Background(), "", "Dopesmoker", 10)
	require.NoError(t, err)
	assert.Nil(t, rels)
	assert.False(t, called, "empty artist must not hit the API")
}

func TestIsDiscogsCoverImage(t *testing.T) {
	assert.True(t, isDiscogsCoverImage("https://i.discogs.com/abc/front.jpeg"))
	assert.False(t, isDiscogsCoverImage("https://st.discogs.com/spacer.gif"), "spacer host rejected")
	assert.False(t, isDiscogsCoverImage("http://i.discogs.com/abc.jpeg"), "non-https rejected")
	assert.False(t, isDiscogsCoverImage("https://evil.test/i.discogs.com"), "look-alike path rejected")
	assert.False(t, isDiscogsCoverImage(""))
}

func TestParseDiscogsYear(t *testing.T) {
	assert.Equal(t, 2003, parseDiscogsYear("2003"))
	assert.Equal(t, 0, parseDiscogsYear(""))
	assert.Equal(t, 0, parseDiscogsYear("0"))
	assert.Equal(t, 0, parseDiscogsYear("n/a"))
}

func TestDiscogsTruncateBody(t *testing.T) {
	short := "short error"
	assert.Equal(t, short, discogsTruncateBody(short))
	long := strings.Repeat("x", discogsErrorBodyLimit+50)
	got := discogsTruncateBody(long)
	assert.True(t, strings.HasSuffix(got, "...[truncated]"))
	assert.Len(t, got, discogsErrorBodyLimit+len("...[truncated]"))
}

func TestDiscogsClient_LimitCapDefaults(t *testing.T) {
	c := newDiscogsTestClient(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "10", r.URL.Query().Get("per_page"), "limit<=0 falls back to the default per_page")
		_, _ = w.Write([]byte(`{"results":[]}`))
	})
	_, err := c.SearchReleaseCovers(context.Background(), "Sleep", "Dopesmoker", 0)
	require.NoError(t, err)
}
