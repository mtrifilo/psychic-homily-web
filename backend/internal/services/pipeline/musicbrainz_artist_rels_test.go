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

func newArtistRelsTestClient(baseURL string) *MusicBrainzClient {
	c := NewMusicBrainzClient()
	c.baseURL = baseURL
	c.rateLimit = time.Millisecond
	return c
}

func TestMusicBrainzClient_LookupArtistArtistRelations(t *testing.T) {
	var gotPath, gotInc string
	mux := http.NewServeMux()
	mux.HandleFunc("/artist/", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotInc = r.URL.Query().Get("inc")
		assert.Equal(t, "json", r.URL.Query().Get("fmt"))
		assert.Equal(t, mbUserAgent, r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"person-1","name":"Thurston Moore","type":"Person",
			"relations":[
				{"type":"member of band","type-id":"5be4c609-9afa-4ea0-910b-12ffb71e3821","direction":"forward","ended":true,"attributes":["guitar","original"],"artist":{"id":"band-1","name":"Sonic Youth","type":"Group"}},
				{"type":"is person","type-id":"dd9886f2-1dfe-4270-97db-283f6839a666","direction":"forward","ended":false,"attributes":[],"artist":{"id":"alias-1","name":"Caribou","type":"Person"}},
				{"type":"married","direction":"forward","ended":true,"attributes":[],"artist":{"id":"person-2","name":"Kim Gordon","type":"Person"}}
			]
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newArtistRelsTestClient(srv.URL)
	rels, err := c.LookupArtistArtistRelations(context.Background(), "person-1")
	require.NoError(t, err)

	assert.Equal(t, "/artist/person-1", gotPath)
	assert.Equal(t, "artist-rels", gotInc)
	require.Len(t, rels, 3)

	assert.Equal(t, "member of band", rels[0].Type)
	assert.Equal(t, "5be4c609-9afa-4ea0-910b-12ffb71e3821", rels[0].TypeID)
	assert.True(t, rels[0].Ended)
	require.NotNil(t, rels[0].Artist)
	assert.Equal(t, "band-1", rels[0].Artist.ID)
	assert.Equal(t, []string{"guitar", "original"}, rels[0].Attributes)

	assert.Equal(t, "is person", rels[1].Type)
	assert.False(t, rels[1].Ended)
	require.NotNil(t, rels[1].Artist)
	assert.Equal(t, "alias-1", rels[1].Artist.ID)
}

func TestMusicBrainzClient_LookupArtistArtistRelations_EscapesID(t *testing.T) {
	var gotRawPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/artist/", func(w http.ResponseWriter, r *http.Request) {
		gotRawPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"relations":[]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newArtistRelsTestClient(srv.URL)
	_, err := c.LookupArtistArtistRelations(context.Background(), "a/b?x=1")
	require.NoError(t, err)
	assert.Contains(t, gotRawPath, "a%2Fb%3Fx=1")
}
