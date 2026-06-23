package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Shared pointer helpers for the Spotify enrichment tests (this and the
// integration suite live in the same package, so these are defined once here).
func spotifyStrPtr(s string) *string { return &s }
func spotifyIntPtr(i int) *int       { return &i }

func TestNormalizeForMatch(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Café Tacvba", "cafe tacvba"},
		{"Sigur Rós", "sigur ros"},
		{"The Velvet Underground & Nico", "the velvet underground nico"},
		{"AC/DC", "ac dc"},
		{"  Multiple   Spaces  ", "multiple spaces"},
		{"Dopesmoker", "dopesmoker"},
		{"MÖTLEY CRÜE", "motley crue"},
		{"", ""},
		{"   ", ""},
		{"...", ""},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, normalizeForMatch(tc.in), "normalizeForMatch(%q)", tc.in)
	}
}

func TestPickStrictAlbumMatch(t *testing.T) {
	cover := []SpotifyImage{{URL: "https://i.scdn.co/image/big", Width: 640, Height: 640}}
	cover2 := []SpotifyImage{{URL: "https://i.scdn.co/image/other", Width: 640, Height: 640}}
	mk := func(name, artist, date string, imgs []SpotifyImage) SpotifyAlbum {
		return SpotifyAlbum{
			Name:         name,
			Artists:      []SpotifyAlbumArtistRef{{Name: artist}},
			ReleaseDate:  date,
			Images:       imgs,
			ExternalURLs: SpotifyExternalURLs{Spotify: "https://open.spotify.com/album/xyz"},
		}
	}
	// pick calls the name-anchored path (no artist Spotify id) and returns just the match.
	pick := func(albums []SpotifyAlbum, artist, title string, year *int) *spotifyAlbumMatch {
		m, _ := pickStrictAlbumMatch(albums, artist, "", title, year)
		return m
	}

	t.Run("exact name+artist, no year → match", func(t *testing.T) {
		m := pick([]SpotifyAlbum{mk("Dopesmoker", "Sleep", "2003", cover)}, "Sleep", "Dopesmoker", nil)
		if assert.NotNil(t, m) {
			assert.Equal(t, "https://i.scdn.co/image/big", m.ImageURL)
			assert.Equal(t, "https://open.spotify.com/album/xyz", m.SourceURL)
		}
	})

	t.Run("case + diacritic insensitive → match", func(t *testing.T) {
		assert.NotNil(t, pick([]SpotifyAlbum{mk("Café Tacvba", "Café Tacvba", "1992", cover)}, "Cafe Tacvba", "cafe tacvba", nil))
	})

	t.Run("album name mismatch → nil", func(t *testing.T) {
		assert.Nil(t, pick([]SpotifyAlbum{mk("Holy Mountain", "Sleep", "2003", cover)}, "Sleep", "Dopesmoker", nil))
	})

	t.Run("artist mismatch → nil", func(t *testing.T) {
		assert.Nil(t, pick([]SpotifyAlbum{mk("Dopesmoker", "Om", "2003", cover)}, "Sleep", "Dopesmoker", nil))
	})

	t.Run("year off by more than tolerance → nil", func(t *testing.T) {
		assert.Nil(t, pick([]SpotifyAlbum{mk("Dopesmoker", "Sleep", "1999", cover)}, "Sleep", "Dopesmoker", spotifyIntPtr(2003)))
	})

	t.Run("year within tolerance → match", func(t *testing.T) {
		assert.NotNil(t, pick([]SpotifyAlbum{mk("Dopesmoker", "Sleep", "2004", cover)}, "Sleep", "Dopesmoker", spotifyIntPtr(2003)))
	})

	t.Run("release year set but candidate year unknown → match (no reject on missing)", func(t *testing.T) {
		assert.NotNil(t, pick([]SpotifyAlbum{mk("Dopesmoker", "Sleep", "", cover)}, "Sleep", "Dopesmoker", spotifyIntPtr(2003)))
	})

	t.Run("qualifying candidate with no image → nil", func(t *testing.T) {
		assert.Nil(t, pick([]SpotifyAlbum{mk("Dopesmoker", "Sleep", "2003", nil)}, "Sleep", "Dopesmoker", nil))
	})

	t.Run("multi-artist album, our artist is featured → still matches", func(t *testing.T) {
		alb := SpotifyAlbum{
			Name:    "Dopesmoker",
			Artists: []SpotifyAlbumArtistRef{{Name: "Sleep"}, {Name: "Guest"}},
			Images:  cover,
		}
		assert.NotNil(t, pick([]SpotifyAlbum{alb}, "Guest", "Dopesmoker", nil))
	})

	t.Run("empty title → nil, count 0", func(t *testing.T) {
		m, n := pickStrictAlbumMatch([]SpotifyAlbum{mk("Dopesmoker", "Sleep", "2003", cover)}, "Sleep", "", "", nil)
		assert.Nil(t, m)
		assert.Equal(t, 0, n)
	})

	t.Run("no candidates → nil, count 0", func(t *testing.T) {
		m, n := pickStrictAlbumMatch(nil, "Sleep", "", "Dopesmoker", nil)
		assert.Nil(t, m)
		assert.Equal(t, 0, n)
	})

	// --- ambiguity (the "skip the rest" guarantee) ---

	t.Run("two qualifiers, different covers, no year → ambiguous nil, count 2", func(t *testing.T) {
		albums := []SpotifyAlbum{
			mk("Dopesmoker", "Sleep", "2003", cover),
			mk("Dopesmoker", "Sleep", "2012", cover2),
		}
		m, n := pickStrictAlbumMatch(albums, "Sleep", "", "Dopesmoker", nil)
		assert.Nil(t, m, "different covers with no year discriminator must skip, not pick #1")
		assert.Equal(t, 2, n)
	})

	t.Run("two qualifiers, SAME cover → not ambiguous, match", func(t *testing.T) {
		albums := []SpotifyAlbum{
			mk("Dopesmoker", "Sleep", "2003", cover),
			mk("Dopesmoker", "Sleep", "2012", cover),
		}
		m, n := pickStrictAlbumMatch(albums, "Sleep", "", "Dopesmoker", nil)
		assert.NotNil(t, m, "identical cover across pressings is not ambiguous")
		assert.Equal(t, 2, n)
	})

	t.Run("two qualifiers, different covers, exact year disambiguates → match the exact-year one", func(t *testing.T) {
		albums := []SpotifyAlbum{
			mk("Dopesmoker", "Sleep", "2003", cover),  // exact year
			mk("Dopesmoker", "Sleep", "2004", cover2), // within tolerance, different cover
		}
		m, _ := pickStrictAlbumMatch(albums, "Sleep", "", "Dopesmoker", spotifyIntPtr(2003))
		if assert.NotNil(t, m) {
			assert.Equal(t, "https://i.scdn.co/image/big", m.ImageURL)
		}
	})

	t.Run("two qualifiers within tolerance, different covers, neither exact → ambiguous nil", func(t *testing.T) {
		albums := []SpotifyAlbum{
			mk("Dopesmoker", "Sleep", "2002", cover),
			mk("Dopesmoker", "Sleep", "2004", cover2),
		}
		m, _ := pickStrictAlbumMatch(albums, "Sleep", "", "Dopesmoker", spotifyIntPtr(2003))
		assert.Nil(t, m, "tolerance-only matches with different covers and no exact year stay ambiguous")
	})

	// --- ID-anchored matching (immune to same-name distinct artists) ---

	t.Run("id-anchored: album artist id matches even when name differs → match", func(t *testing.T) {
		alb := SpotifyAlbum{
			Name:         "Dopesmoker",
			Artists:      []SpotifyAlbumArtistRef{{ID: "SLEEP_ID", Name: "Sleep (Official)"}},
			Images:       cover,
			ExternalURLs: SpotifyExternalURLs{Spotify: "https://open.spotify.com/album/xyz"},
		}
		m, _ := pickStrictAlbumMatch([]SpotifyAlbum{alb}, "Sleep", "SLEEP_ID", "Dopesmoker", nil)
		assert.NotNil(t, m)
	})

	t.Run("id-anchored: wrong artist id but matching name → nil (id required when anchoring)", func(t *testing.T) {
		alb := SpotifyAlbum{
			Name:    "Dopesmoker",
			Artists: []SpotifyAlbumArtistRef{{ID: "OTHER_ID", Name: "Sleep"}},
			Images:  cover,
		}
		m, _ := pickStrictAlbumMatch([]SpotifyAlbum{alb}, "Sleep", "SLEEP_ID", "Dopesmoker", nil)
		assert.Nil(t, m, "a same-named but different-id artist must not match when anchoring by id")
	})
}

func TestExtractSpotifyArtistID(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://open.spotify.com/artist/3o2dn2O0FCVsWDFSh8qxgG", "3o2dn2O0FCVsWDFSh8qxgG"},
		{"https://open.spotify.com/artist/ABC123/about", "ABC123"},
		{"https://open.spotify.com/intl-de/artist/ABC123", "ABC123"},
		{"https://open.spotify.com/intl-de/artist/ABC123/about", "ABC123"},
		{"https://open.spotify.com/artist/ABC123?si=xyz", "ABC123"},
		{"  https://open.spotify.com/artist/ABC123  ", "ABC123"},        // trimmed
		{"https://open.spotify.com/embed/artist/ABC123", "ABC123"},      // embed prefix
		{"https://open.spotify.com/artist/FIRST/artist/SECOND", "FIRST"}, // first artist segment wins
		// Rejected: look-alike host (SSRF-style), wrong path, no scheme, junk.
		{"https://open.spotify.com/user/artist/foo", ""}, // user literally named "artist" → not an artist link
		{"https://evil.test/artist/ABC123", ""},
		{"https://open.spotify.com.evil.test/artist/ABC123", ""},
		{"https://open.spotify.com@evil.test/artist/ABC123", ""}, // userinfo trick → host is evil.test
		{"https://open.spotify.com/album/XYZ", ""},
		{"open.spotify.com/artist/ABC123", ""},   // no scheme → empty host
		{"//open.spotify.com/artist/ABC123", ""}, // scheme-relative → rejected by scheme guard
		{"ftp://open.spotify.com/artist/ABC123", ""}, // non-http scheme → rejected
		{"https://open.spotify.com/artist/", ""},
		{"", ""},
		{"   ", ""},
		{"https://open.spotify.com/artist/bad id", ""}, // non-alphanumeric id
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, extractSpotifyArtistID(tc.in), "extractSpotifyArtistID(%q)", tc.in)
	}
}

func TestBestImageURL(t *testing.T) {
	t.Run("picks widest", func(t *testing.T) {
		imgs := []SpotifyImage{
			{URL: "small", Width: 64},
			{URL: "big", Width: 640},
			{URL: "med", Width: 300},
		}
		assert.Equal(t, "big", bestImageURL(imgs))
	})
	t.Run("skips empty-url entries", func(t *testing.T) {
		imgs := []SpotifyImage{{URL: "", Width: 640}, {URL: "real", Width: 64}}
		assert.Equal(t, "real", bestImageURL(imgs))
	})
	t.Run("empty slice → empty", func(t *testing.T) {
		assert.Equal(t, "", bestImageURL(nil))
	})
}

func TestPrimaryArtistForMatch(t *testing.T) {
	t.Run("empty → empty", func(t *testing.T) {
		name, id := primaryArtistForMatch(nil)
		assert.Equal(t, "", name)
		assert.Equal(t, "", id)
	})
	t.Run("prefers an artist with a parseable spotify link (enables id-anchoring)", func(t *testing.T) {
		artists := []catalogm.Artist{
			{Name: "NoLink"},
			{Name: "Linked", Social: catalogm.Social{Spotify: spotifyStrPtr("https://open.spotify.com/artist/ABC123")}},
		}
		name, id := primaryArtistForMatch(artists)
		assert.Equal(t, "Linked", name)
		assert.Equal(t, "ABC123", id)
	})
	t.Run("falls back to first non-empty name when no link", func(t *testing.T) {
		artists := []catalogm.Artist{{Name: "   "}, {Name: "First"}, {Name: "Second"}}
		name, id := primaryArtistForMatch(artists)
		assert.Equal(t, "First", name)
		assert.Equal(t, "", id)
	})
	t.Run("link present but unparseable → name fallback, no id", func(t *testing.T) {
		artists := []catalogm.Artist{{Name: "Band", Social: catalogm.Social{Spotify: spotifyStrPtr("https://evil.test/artist/x")}}}
		name, id := primaryArtistForMatch(artists)
		assert.Equal(t, "Band", name)
		assert.Equal(t, "", id)
	})
}

func TestIsHTTPSURL(t *testing.T) {
	assert.True(t, isHTTPSURL("https://i.scdn.co/image/abc"))
	assert.True(t, isHTTPSURL("https://mosaic.scdn.co/640/abc"))
	assert.False(t, isHTTPSURL("http://i.scdn.co/image/abc"), "http is not storable")
	assert.False(t, isHTTPSURL("javascript:alert(1)"))
	assert.False(t, isHTTPSURL("data:image/png;base64,AAAA"))
	assert.False(t, isHTTPSURL(""))
	assert.False(t, isHTTPSURL("https://"), "no host")
}

func TestIsSpotifyWebURL(t *testing.T) {
	assert.True(t, isSpotifyWebURL("https://open.spotify.com/album/abc"))
	assert.True(t, isSpotifyWebURL("https://open.spotify.com/artist/abc"))
	assert.False(t, isSpotifyWebURL("http://open.spotify.com/album/abc"), "must be https")
	assert.False(t, isSpotifyWebURL("https://evil.test/album/abc"))
	assert.False(t, isSpotifyWebURL("https://open.spotify.com.evil.test/album/abc"))
	assert.False(t, isSpotifyWebURL("javascript:alert(1)"))
	assert.False(t, isSpotifyWebURL(""))
}

func TestAbsInt(t *testing.T) {
	assert.Equal(t, 3, absInt(-3))
	assert.Equal(t, 3, absInt(3))
	assert.Equal(t, 0, absInt(0))
}
