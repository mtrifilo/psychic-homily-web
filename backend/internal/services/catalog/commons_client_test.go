package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCommonsTestClient(t *testing.T, handler http.HandlerFunc) *CommonsClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewCommonsClientWithConfig(srv.Client(), srv.URL)
	t.Cleanup(c.Close)
	return c
}

const commonsFoundBody = `{"query":{"pages":{"123":{"title":"File:X.jpg","imageinfo":[{
	"url":"https://upload.wikimedia.org/wikipedia/commons/a/ab/X.jpg",
	"descriptionurl":"https://commons.wikimedia.org/wiki/File:X.jpg",
	"thumburl":"https://upload.wikimedia.org/wikipedia/commons/thumb/a/ab/X.jpg/600px-X.jpg",
	"extmetadata":{"LicenseShortName":{"value":"CC BY-SA 4.0"},"Artist":{"value":"<a href=\"//x\">Jane Doe</a>"}}
}]}}}}`

func TestCommonsClient_ImageInfo_Found(t *testing.T) {
	c := newCommonsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/w/api.php", r.URL.Path)
		q := r.URL.Query()
		assert.Equal(t, "query", q.Get("action"))
		assert.Equal(t, "File:X.jpg", q.Get("titles"))
		assert.Equal(t, "imageinfo", q.Get("prop"))
		assert.Equal(t, "600", q.Get("iiurlwidth"))
		assert.Equal(t, commonsUserAgent, r.Header.Get("User-Agent"))
		_, _ = w.Write([]byte(commonsFoundBody))
	})

	img, err := c.ImageInfo(context.Background(), "X.jpg")
	require.NoError(t, err)
	require.NotNil(t, img)
	// Prefers the thumbnail over the full-res original.
	assert.Equal(t, "https://upload.wikimedia.org/wikipedia/commons/thumb/a/ab/X.jpg/600px-X.jpg", img.ImageURL)
	assert.Equal(t, "https://commons.wikimedia.org/wiki/File:X.jpg", img.DescriptionURL)
	assert.Equal(t, "CC BY-SA 4.0", img.License)
	assert.Equal(t, "Jane Doe", img.Author, "author HTML is stripped")
}

func TestCommonsClient_NonFreeLicenseDropped(t *testing.T) {
	c := newCommonsTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"query":{"pages":{"1":{"imageinfo":[{
			"thumburl":"https://upload.wikimedia.org/x.jpg",
			"descriptionurl":"https://commons.wikimedia.org/wiki/File:X.jpg",
			"extmetadata":{"LicenseShortName":{"value":"Fair use"}}}]}}}}`))
	})
	img, err := c.ImageInfo(context.Background(), "X.jpg")
	require.NoError(t, err)
	assert.Nil(t, img, "a non-reusable license is dropped (fail-closed)")
}

func TestCommonsClient_MissingFile(t *testing.T) {
	c := newCommonsTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"query":{"pages":{"-1":{"title":"File:Nope.jpg","missing":""}}}}`))
	})
	img, err := c.ImageInfo(context.Background(), "Nope.jpg")
	require.NoError(t, err)
	assert.Nil(t, img)
}

func TestCommonsClient_EmptyFilenameNoCall(t *testing.T) {
	var called bool
	c := newCommonsTestClient(t, func(_ http.ResponseWriter, _ *http.Request) { called = true })
	_, err := c.ImageInfo(context.Background(), "  ")
	require.Error(t, err)
	assert.False(t, called)
}

func TestCommonsClient_ServerError(t *testing.T) {
	c := newCommonsTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	_, err := c.ImageInfo(context.Background(), "X.jpg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestIsReusableLicense(t *testing.T) {
	for _, ok := range []string{"CC BY 2.0", "CC BY-SA 4.0", "CC0", "Public domain", "PDM", "PD-old"} {
		assert.True(t, isReusableLicense(ok), ok)
	}
	for _, no := range []string{"", "Fair use", "All rights reserved", "GFDL", "Copyrighted",
		// NonCommercial / NoDerivatives variants must be rejected despite starting "CC BY".
		"CC BY-NC 4.0", "CC BY-ND 2.0", "CC BY-NC-SA 3.0", "CC BY-NC-ND 4.0"} {
		assert.False(t, isReusableLicense(no), no)
	}
}

func TestStripCommonsHTML(t *testing.T) {
	assert.Equal(t, "Jane Doe", stripCommonsHTML(`<a href="//x">Jane Doe</a>`))
	assert.Equal(t, "Tom & Jerry", stripCommonsHTML(`<span>Tom &amp; Jerry</span>`))
	assert.Equal(t, "Plain Name", stripCommonsHTML("Plain Name"))
	assert.Equal(t, "", stripCommonsHTML(""))
	// An entity-encoded tag is unescaped THEN stripped, so no markup is stored.
	assert.Equal(t, "x", stripCommonsHTML("&lt;script&gt;x&lt;/script&gt;"))
}
