package services

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetch_200_Changed(t *testing.T) {
	body := "<html><body>Hello World</body></html>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}))
	defer server.Close()

	svc := NewFetcherService()
	result, err := svc.Fetch(server.URL, "", "")

	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Equal(t, body, result.Body)
	assert.Equal(t, http.StatusOK, result.HTTPStatus)
	assert.NotEmpty(t, result.ContentHash)
	assert.Equal(t, "text/html; charset=utf-8", result.ContentType)
}

func TestFetch_200_Unchanged(t *testing.T) {
	body := "<html><body>Same content</body></html>"
	hash := computeContentHash(body)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}))
	defer server.Close()

	svc := NewFetcherService()
	result, err := svc.Fetch(server.URL, "", hash)

	require.NoError(t, err)
	assert.False(t, result.Changed)
	assert.Empty(t, result.Body, "Body should be empty when content is unchanged")
	assert.Equal(t, http.StatusOK, result.HTTPStatus)
	assert.Equal(t, hash, result.ContentHash)
}

func TestFetch_200_HashMismatch(t *testing.T) {
	body := "<html><body>New content</body></html>"
	oldHash := computeContentHash("<html><body>Old content</body></html>")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}))
	defer server.Close()

	svc := NewFetcherService()
	result, err := svc.Fetch(server.URL, "", oldHash)

	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Equal(t, body, result.Body)
	assert.NotEqual(t, oldHash, result.ContentHash)
}

func TestFetch_304_NotModified(t *testing.T) {
	etag := `"abc123"`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("body"))
	}))
	defer server.Close()

	svc := NewFetcherService()
	result, err := svc.Fetch(server.URL, etag, "")

	require.NoError(t, err)
	assert.False(t, result.Changed)
	assert.Equal(t, http.StatusNotModified, result.HTTPStatus)
	assert.Empty(t, result.Body)
}

func TestFetch_ETag_Sent(t *testing.T) {
	etag := `"my-etag-value"`
	var receivedETag string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedETag = r.Header.Get("If-None-Match")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("body"))
	}))
	defer server.Close()

	svc := NewFetcherService()
	_, err := svc.Fetch(server.URL, etag, "")

	require.NoError(t, err)
	assert.Equal(t, etag, receivedETag)
}

func TestFetch_ETag_NotSent_WhenEmpty(t *testing.T) {
	var receivedETag string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedETag = r.Header.Get("If-None-Match")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("body"))
	}))
	defer server.Close()

	svc := NewFetcherService()
	_, err := svc.Fetch(server.URL, "", "")

	require.NoError(t, err)
	assert.Empty(t, receivedETag, "If-None-Match should not be sent when lastETag is empty")
}

func TestFetch_301_Redirect(t *testing.T) {
	newURL := "https://newvenue.example.com/calendar"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", newURL)
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer server.Close()

	svc := NewFetcherService()
	result, err := svc.Fetch(server.URL, "", "")

	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Equal(t, http.StatusMovedPermanently, result.HTTPStatus)
	assert.Equal(t, newURL, result.RedirectURL)
	assert.Empty(t, result.Body)
}

func TestFetch_308_Redirect(t *testing.T) {
	newURL := "https://newvenue.example.com/events"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", newURL)
		w.WriteHeader(http.StatusPermanentRedirect)
	}))
	defer server.Close()

	svc := NewFetcherService()
	result, err := svc.Fetch(server.URL, "", "")

	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Equal(t, http.StatusPermanentRedirect, result.HTTPStatus)
	assert.Equal(t, newURL, result.RedirectURL)
}

func TestFetch_403_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	svc := NewFetcherService()
	result, err := svc.Fetch(server.URL, "", "")

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

func TestFetch_429_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	svc := NewFetcherService()
	result, err := svc.Fetch(server.URL, "", "")

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")
}

func TestFetch_500_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	svc := NewFetcherService()
	result, err := svc.Fetch(server.URL, "", "")

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestFetch_502_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	svc := NewFetcherService()
	result, err := svc.Fetch(server.URL, "", "")

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "502")
}

func TestFetch_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hang longer than the client timeout
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Use a very short timeout for the test
	svc := NewFetcherServiceWithTimeout(100 * time.Millisecond)
	result, err := svc.Fetch(server.URL, "", "")

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching")
}

func TestFetch_InvalidURL(t *testing.T) {
	svc := NewFetcherService()
	result, err := svc.Fetch("http://this-domain-does-not-exist-xyz.invalid", "", "")

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching")
}

func TestComputeContentHash(t *testing.T) {
	hash := computeContentHash("hello world")
	// SHA256 of "hello world"
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	assert.Equal(t, expected, hash)
}

func TestComputeContentHash_Deterministic(t *testing.T) {
	content := "<html><body>Test content for hashing</body></html>"
	hash1 := computeContentHash(content)
	hash2 := computeContentHash(content)
	assert.Equal(t, hash1, hash2, "Same input should produce same hash")
}

func TestComputeContentHash_DifferentContent(t *testing.T) {
	hash1 := computeContentHash("content A")
	hash2 := computeContentHash("content B")
	assert.NotEqual(t, hash1, hash2, "Different input should produce different hash")
}

func TestFetch_UserAgent(t *testing.T) {
	var receivedUA string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("body"))
	}))
	defer server.Close()

	svc := NewFetcherService()
	_, err := svc.Fetch(server.URL, "", "")

	require.NoError(t, err)
	assert.Equal(t, "PsychicHomily/1.0 (venue-calendar-indexer)", receivedUA)
}

func TestFetch_ContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"events":[]}`))
	}))
	defer server.Close()

	svc := NewFetcherService()
	result, err := svc.Fetch(server.URL, "", "")

	require.NoError(t, err)
	assert.Equal(t, "application/json; charset=utf-8", result.ContentType)
}

func TestFetch_200_WithETag(t *testing.T) {
	body := "<html>events</html>"
	responseETag := `"v2-etag"`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", responseETag)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}))
	defer server.Close()

	svc := NewFetcherService()
	result, err := svc.Fetch(server.URL, "", "")

	require.NoError(t, err)
	assert.Equal(t, responseETag, result.ETag)
	assert.True(t, result.Changed)
	assert.Equal(t, body, result.Body)
}
