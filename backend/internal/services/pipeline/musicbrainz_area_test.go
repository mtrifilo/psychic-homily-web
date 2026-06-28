package pipeline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAreaTestClient points a client at an httptest server with a fast throttle
// so the area-lookup tests need no network and no ~1s wait.
func newAreaTestClient(baseURL string) *MusicBrainzClient {
	c := NewMusicBrainzClient()
	c.baseURL = baseURL
	c.rateLimit = time.Millisecond
	return c
}

func TestMusicBrainzClient_LookupAreaRelations(t *testing.T) {
	var gotPath, gotInc string
	mux := http.NewServeMux()
	mux.HandleFunc("/area/", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotInc = r.URL.Query().Get("inc")
		assert.Equal(t, "json", r.URL.Query().Get("fmt"))
		assert.Equal(t, mbUserAgent, r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		// A City "part of" both a County and the parent Subdivision (California).
		// The state is identified by Area.Type, not Direction.
		_, _ = w.Write([]byte(`{"id":"city-1","name":"Pasadena","type":"City","relations":[
			{"type":"part of","direction":"backward","area":{"id":"county-1","name":"Los Angeles County","type":"County"}},
			{"type":"part of","direction":"backward","area":{"id":"subdiv-1","name":"California","type":"Subdivision"}}
		]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newAreaTestClient(srv.URL)
	rels, err := c.LookupAreaRelations(context.Background(), "city-1")
	require.NoError(t, err)

	assert.Equal(t, "/area/city-1", gotPath)
	assert.Equal(t, "area-rels", gotInc)
	require.Len(t, rels, 2)

	// The Subdivision relation carries the parent state name.
	var subdiv *MBAreaRelation
	for i := range rels {
		if rels[i].Area != nil && strings.EqualFold(rels[i].Area.Type, "Subdivision") {
			subdiv = &rels[i]
		}
	}
	require.NotNil(t, subdiv, "expected a Subdivision relation")
	assert.Equal(t, "California", subdiv.Area.Name)
}

// TestMusicBrainzClient_LookupAreaRelations_EscapesID verifies a malformed area
// ID can't alter the request target (path-escaped, like the artist lookup).
func TestMusicBrainzClient_LookupAreaRelations_EscapesID(t *testing.T) {
	var gotRawPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/area/", func(w http.ResponseWriter, r *http.Request) {
		gotRawPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"relations":[]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newAreaTestClient(srv.URL)
	_, err := c.LookupAreaRelations(context.Background(), "a/b?x=1")
	require.NoError(t, err)
	assert.Contains(t, gotRawPath, "a%2Fb%3Fx=1")
}
