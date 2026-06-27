package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

func TestExtractWikidataQID(t *testing.T) {
	assert.Equal(t, "Q42", extractWikidataQID([]string{"https://www.wikidata.org/wiki/Q42"}))
	assert.Equal(t, "Q42", extractWikidataQID([]string{"https://www.wikidata.org/entity/Q42"}))
	assert.Equal(t, "Q42", extractWikidataQID([]string{"https://band.bandcamp.com", "http://www.wikidata.org/wiki/Q42"}))
	assert.Equal(t, "", extractWikidataQID([]string{"https://example.com/Q42"}))
	assert.Equal(t, "", extractWikidataQID(nil))
}

func TestNormalizeLink(t *testing.T) {
	assert.Equal(t, "artist.bandcamp.com", normalizeLink("https://Artist.bandcamp.com/"))
	assert.Equal(t, "artist.bandcamp.com", normalizeLink("http://artist.bandcamp.com"))
	assert.Equal(t, "open.spotify.com/artist/abc", normalizeLink("https://open.spotify.com/artist/abc"))
	assert.Equal(t, "example.com", normalizeLink("https://www.example.com/"))
	assert.Equal(t, "", normalizeLink("not a url"))
	assert.Equal(t, "", normalizeLink(""))
}

func TestArtistLinkKeys(t *testing.T) {
	ar := &catalogm.Artist{Social: catalogm.Social{
		Spotify:  stringPtr("https://open.spotify.com/artist/abc"),
		Bandcamp: stringPtr("https://band.bandcamp.com"),
		Website:  stringPtr("https://band.example/"),
	}}
	keys := artistLinkKeys(ar)
	assert.Contains(t, keys, "open.spotify.com/artist/abc")
	assert.Contains(t, keys, "band.bandcamp.com")
	assert.Contains(t, keys, "band.example")
	assert.Len(t, keys, 3, "nil social fields (SoundCloud) are skipped")
}

func TestUrlsShareLink(t *testing.T) {
	anchors := []string{"open.spotify.com/artist/abc", "band.bandcamp.com"}
	assert.True(t, urlsShareLink([]string{"https://band.bandcamp.com/"}, anchors), "bandcamp subdomain identity")
	assert.True(t, urlsShareLink([]string{"https://open.spotify.com/artist/abc"}, anchors), "same spotify artist")
	assert.False(t, urlsShareLink([]string{"https://open.spotify.com/artist/XYZ"}, anchors),
		"a DIFFERENT spotify artist on the same host must not anchor")
	assert.False(t, urlsShareLink([]string{"https://other.bandcamp.com"}, anchors))
	assert.False(t, urlsShareLink(nil, anchors))
}

func TestValidCommonsImageURL(t *testing.T) {
	assert.True(t, validCommonsImageURL("https://upload.wikimedia.org/x/600px-X.jpg"))
	assert.False(t, validCommonsImageURL("http://upload.wikimedia.org/x.jpg"), "non-https rejected")
	assert.False(t, validCommonsImageURL("https://evil.test/x.jpg"))
	assert.False(t, validCommonsImageURL("https://upload.wikimedia.org.attacker.test/x.jpg"))
}

func TestIsCommonsWebURL(t *testing.T) {
	assert.True(t, isCommonsWebURL("https://commons.wikimedia.org/wiki/File:X.jpg"))
	assert.False(t, isCommonsWebURL("http://commons.wikimedia.org/wiki/File:X.jpg"))
	assert.False(t, isCommonsWebURL("https://upload.wikimedia.org/x.jpg"), "image host is not the web host")
	assert.False(t, isCommonsWebURL("https://commons.wikimedia.org.attacker.test/x"))
}

func TestDedupOne(t *testing.T) {
	assert.True(t, dedupOne([]string{"Q1"}))
	assert.True(t, dedupOne([]string{"Q1", "Q1"}))
	assert.False(t, dedupOne([]string{"Q1", "Q2"}), "conflicting anchors do not resolve")
	assert.False(t, dedupOne([]string{}))
	assert.False(t, dedupOne([]string{"", ""}))
}

func TestNilIfEmpty(t *testing.T) {
	assert.Nil(t, nilIfEmpty(""))
	assert.Nil(t, nilIfEmpty("   "))
	assert.Equal(t, "x", nilIfEmpty("x"))
}
