package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newWikidataTestClient(t *testing.T, handler http.HandlerFunc) *WikidataClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewWikidataClientWithConfig(srv.Client(), srv.URL)
	t.Cleanup(c.Close)
	return c
}

func TestWikidataClient_ImageFilename(t *testing.T) {
	c := newWikidataTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/w/api.php", r.URL.Path)
		q := r.URL.Query()
		assert.Equal(t, "wbgetclaims", q.Get("action"))
		assert.Equal(t, "Q123", q.Get("entity"))
		assert.Equal(t, "P18", q.Get("property"))
		assert.Equal(t, wikidataUserAgent, r.Header.Get("User-Agent"))
		_, _ = w.Write([]byte(`{"claims":{"P18":[{"mainsnak":{"datavalue":{"value":"Jane Doe.jpg","type":"string"}}}]}}`))
	})

	fn, err := c.ImageFilename(context.Background(), "Q123")
	require.NoError(t, err)
	assert.Equal(t, "Jane Doe.jpg", fn)
}

func TestWikidataClient_NoImageClaim(t *testing.T) {
	c := newWikidataTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"claims":{}}`))
	})
	fn, err := c.ImageFilename(context.Background(), "Q123")
	require.NoError(t, err)
	assert.Equal(t, "", fn, "no P18 claim is a normal no-image outcome")
}

func TestWikidataClient_EmptyQIDNoCall(t *testing.T) {
	var called bool
	c := newWikidataTestClient(t, func(_ http.ResponseWriter, _ *http.Request) { called = true })
	_, err := c.ImageFilename(context.Background(), "  ")
	require.Error(t, err)
	assert.False(t, called, "empty qid must not hit the API")
}

func TestWikidataClient_ServerError(t *testing.T) {
	c := newWikidataTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	_, err := c.ImageFilename(context.Background(), "Q123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}
