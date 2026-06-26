package catalog

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spotifyTestServer wires a single httptest server that plays the token,
// search, and artist endpoints, with per-endpoint call counters.
type spotifyTestServer struct {
	srv         *httptest.Server
	tokenCalls  atomic.Int32
	searchCalls atomic.Int32
	artistCalls atomic.Int32
	// search429Once: when true, the first /search returns 429 then succeeds.
	search429Once atomic.Bool
}

func newSpotifyTestServer(t *testing.T) *spotifyTestServer {
	t.Helper()
	ts := &spotifyTestServer{}
	mux := http.NewServeMux()

	mux.HandleFunc("/api/token", func(w http.ResponseWriter, r *http.Request) {
		ts.tokenCalls.Add(1)
		user, pass, ok := r.BasicAuth()
		assert.True(t, ok, "token request must use basic auth")
		assert.Equal(t, "id", user)
		assert.Equal(t, "secret", pass)
		assert.Equal(t, "client_credentials", r.FormValue("grant_type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-abc","token_type":"Bearer","expires_in":3600}`))
	})

	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer tok-abc", r.Header.Get("Authorization"))
		if ts.search429Once.CompareAndSwap(true, false) {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"status":429}}`))
			return
		}
		ts.searchCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"albums":{"items":[
			{"id":"alb1","name":"Dopesmoker","artists":[{"id":"ABC123","name":"Sleep"}],"release_date":"2003",
			 "images":[{"url":"https://i.scdn.co/image/big","width":640,"height":640}],
			 "external_urls":{"spotify":"https://open.spotify.com/album/alb1"}}
		]}}`))
	})

	mux.HandleFunc("/artists/", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer tok-abc", r.Header.Get("Authorization"))
		ts.artistCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ABC123","name":"Sleep",
			"images":[{"url":"https://i.scdn.co/artist/big","width":640,"height":640}],
			"external_urls":{"spotify":"https://open.spotify.com/artist/ABC123"}}`))
	})

	ts.srv = httptest.NewServer(mux)
	t.Cleanup(ts.srv.Close)
	return ts
}

func (ts *spotifyTestServer) client() *SpotifyClient {
	return NewSpotifyClientWithConfig(ts.srv.Client(), ts.srv.URL, ts.srv.URL, "id", "secret")
}

func TestSpotifyClient_SearchAlbums(t *testing.T) {
	ts := newSpotifyTestServer(t)
	c := ts.client()
	defer c.Close()

	albums, err := c.SearchAlbums("Sleep", "Dopesmoker", 10)
	require.NoError(t, err)
	require.Len(t, albums, 1)
	assert.Equal(t, "Dopesmoker", albums[0].Name)
	require.Len(t, albums[0].Artists, 1)
	assert.Equal(t, "Sleep", albums[0].Artists[0].Name)
	assert.Equal(t, "ABC123", albums[0].Artists[0].ID)
	assert.Equal(t, "2003", albums[0].ReleaseDate)
	assert.Equal(t, "https://i.scdn.co/image/big", bestImageURL(albums[0].Images))
	assert.Equal(t, "https://open.spotify.com/album/alb1", albums[0].ExternalURLs.Spotify)
}

func TestSpotifyClient_SearchSendsFieldFilteredQuery(t *testing.T) {
	ts := newSpotifyTestServer(t)
	var gotQuery string
	// Replace the search handler to capture the query.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/token", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"tok-abc","expires_in":3600}`))
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		assert.Equal(t, "album", r.URL.Query().Get("type"))
		_, _ = w.Write([]byte(`{"albums":{"items":[]}}`))
	})
	ts.srv.Config.Handler = mux

	c := ts.client()
	defer c.Close()
	_, err := c.SearchAlbums("Sleep", "Dopesmoker", 10)
	require.NoError(t, err)
	assert.Contains(t, gotQuery, `album:"Dopesmoker"`)
	assert.Contains(t, gotQuery, `artist:"Sleep"`)
}

func TestSpotifyClient_TokenCachedAcrossCalls(t *testing.T) {
	ts := newSpotifyTestServer(t)
	c := ts.client()
	defer c.Close()

	_, err := c.SearchAlbums("Sleep", "Dopesmoker", 10)
	require.NoError(t, err)
	_, err = c.GetArtist("ABC123")
	require.NoError(t, err)

	assert.Equal(t, int32(1), ts.tokenCalls.Load(), "token should be fetched once and cached")
	assert.Equal(t, int32(1), ts.searchCalls.Load())
	assert.Equal(t, int32(1), ts.artistCalls.Load())
}

func TestSpotifyClient_GetArtist(t *testing.T) {
	ts := newSpotifyTestServer(t)
	c := ts.client()
	defer c.Close()

	a, err := c.GetArtist("ABC123")
	require.NoError(t, err)
	require.NotNil(t, a)
	assert.Equal(t, "Sleep", a.Name)
	assert.Equal(t, "https://i.scdn.co/artist/big", bestImageURL(a.Images))
	assert.Equal(t, "https://open.spotify.com/artist/ABC123", a.ExternalURLs.Spotify)
}

func TestSpotifyClient_GetArtist_EmptyID(t *testing.T) {
	ts := newSpotifyTestServer(t)
	c := ts.client()
	defer c.Close()

	_, err := c.GetArtist("   ")
	require.Error(t, err)
	assert.Equal(t, int32(0), ts.artistCalls.Load(), "empty id must not hit the API")
}

func TestSpotifyClient_EmptyArgsNoCall(t *testing.T) {
	ts := newSpotifyTestServer(t)
	c := ts.client()
	defer c.Close()

	albums, err := c.SearchAlbums("", "Dopesmoker", 10)
	require.NoError(t, err)
	assert.Nil(t, albums)
	assert.Equal(t, int32(0), ts.tokenCalls.Load(), "empty artist/title must not hit the API")
}

func TestSpotifyClient_429ThenSuccess(t *testing.T) {
	ts := newSpotifyTestServer(t)
	ts.search429Once.Store(true)
	c := ts.client()
	defer c.Close()

	albums, err := c.SearchAlbums("Sleep", "Dopesmoker", 10)
	require.NoError(t, err, "client should retry once after a 429 and succeed")
	assert.Len(t, albums, 1)
	assert.Equal(t, int32(1), ts.searchCalls.Load(), "the successful retry counts once")
}

func TestSpotifyClient_Non200Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/token":
			_, _ = w.Write([]byte(`{"access_token":"tok-abc","expires_in":3600}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"status":500,"message":"boom"}}`))
		}
	}))
	defer srv.Close()

	c := NewSpotifyClientWithConfig(srv.Client(), srv.URL, srv.URL, "id", "secret")
	defer c.Close()

	_, err := c.SearchAlbums("Sleep", "Dopesmoker", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestSpotifyClient_TokenError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
			return
		}
		_, _ = w.Write([]byte(`{"albums":{"items":[]}}`))
	}))
	defer srv.Close()

	c := NewSpotifyClientWithConfig(srv.Client(), srv.URL, srv.URL, "id", "bad")
	defer c.Close()

	_, err := c.SearchAlbums("Sleep", "Dopesmoker", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token endpoint returned status 401")
}

func TestSpotifyClient_TokenRefreshOnExpiry(t *testing.T) {
	// expires_in:0 → the cached token is already expired, so every API call
	// re-fetches a token (exercises the expiry branch in ensureToken).
	var tokenCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/token", func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		_, _ = w.Write([]byte(`{"access_token":"tok-abc","expires_in":0}`))
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"albums":{"items":[]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewSpotifyClientWithConfig(srv.Client(), srv.URL, srv.URL, "id", "secret")
	defer c.Close()

	_, err := c.SearchAlbums("Sleep", "Dopesmoker", 10)
	require.NoError(t, err)
	_, err = c.SearchAlbums("Sleep", "Holy Mountain", 10)
	require.NoError(t, err)
	assert.Equal(t, int32(2), tokenCalls.Load(), "an expired token is re-fetched per call")
}

func TestParseSpotifyRetryAfter(t *testing.T) {
	assert.Equal(t, 5*time.Second, parseSpotifyRetryAfter("5"), "sub-cap delta passes through")
	assert.Equal(t, 90*time.Second, parseSpotifyRetryAfter("90"), "sub-cap delta passes through")
	assert.Equal(t, time.Duration(0), parseSpotifyRetryAfter("0"))
	assert.Equal(t, time.Duration(0), parseSpotifyRetryAfter("-5"), "negative delta floors to 0")
	assert.Equal(t, spotify429MaxWait, parseSpotifyRetryAfter("99999"), "delta over the cap clamps to max")
	assert.Equal(t, spotify429DefaultBackoff, parseSpotifyRetryAfter(""), "missing header → default backoff")
	assert.Equal(t, spotify429DefaultBackoff, parseSpotifyRetryAfter("not-a-number"), "unparseable → default backoff")

	// HTTP-date form (RFC 9110): a far-future date clamps to the max; a past date floors to 0.
	assert.Equal(t, spotify429MaxWait, parseSpotifyRetryAfter("Wed, 21 Oct 2099 07:28:00 GMT"))
	assert.Equal(t, time.Duration(0), parseSpotifyRetryAfter("Wed, 21 Oct 1999 07:28:00 GMT"))
}

func TestSpotifyClient_PersistentThrottleReturnsSentinel(t *testing.T) {
	// Every request 429s with Retry-After:0 (so retries are instant) → after the
	// retry budget, apiGet returns ErrSpotifyRateLimited rather than spinning.
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok-abc","expires_in":3600}`))
			return
		}
		calls.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"status":429}}`))
	}))
	defer srv.Close()

	c := NewSpotifyClientWithConfig(srv.Client(), srv.URL, srv.URL, "id", "secret")
	defer c.Close()

	_, err := c.SearchAlbums("Sleep", "Dopesmoker", 10)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSpotifyRateLimited), "persistent 429 must surface ErrSpotifyRateLimited")
	assert.Equal(t, int32(spotify429MaxRetries+1), calls.Load(), "1 initial + spotify429MaxRetries attempts")
}
