package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	mk := func(name, artist, date string, imgs []SpotifyImage) SpotifyAlbum {
		return SpotifyAlbum{
			Name:         name,
			Artists:      []SpotifyAlbumArtistRef{{Name: artist}},
			ReleaseDate:  date,
			Images:       imgs,
			ExternalURLs: SpotifyExternalURLs{Spotify: "https://open.spotify.com/album/xyz"},
		}
	}

	t.Run("exact name+artist, no year → match", func(t *testing.T) {
		m := pickStrictAlbumMatch([]SpotifyAlbum{mk("Dopesmoker", "Sleep", "2003", cover)}, "Sleep", "Dopesmoker", nil)
		if assert.NotNil(t, m) {
			assert.Equal(t, "https://i.scdn.co/image/big", m.ImageURL)
			assert.Equal(t, "https://open.spotify.com/album/xyz", m.SourceURL)
		}
	})

	t.Run("case + diacritic insensitive → match", func(t *testing.T) {
		m := pickStrictAlbumMatch([]SpotifyAlbum{mk("Café Tacvba", "Café Tacvba", "1992", cover)}, "Cafe Tacvba", "cafe tacvba", nil)
		assert.NotNil(t, m)
	})

	t.Run("album name mismatch → nil", func(t *testing.T) {
		m := pickStrictAlbumMatch([]SpotifyAlbum{mk("Holy Mountain", "Sleep", "2003", cover)}, "Sleep", "Dopesmoker", nil)
		assert.Nil(t, m)
	})

	t.Run("artist mismatch → nil", func(t *testing.T) {
		m := pickStrictAlbumMatch([]SpotifyAlbum{mk("Dopesmoker", "Om", "2003", cover)}, "Sleep", "Dopesmoker", nil)
		assert.Nil(t, m)
	})

	t.Run("year off by more than tolerance → nil", func(t *testing.T) {
		m := pickStrictAlbumMatch([]SpotifyAlbum{mk("Dopesmoker", "Sleep", "1999", cover)}, "Sleep", "Dopesmoker", spotifyIntPtr(2003))
		assert.Nil(t, m)
	})

	t.Run("year within tolerance → match", func(t *testing.T) {
		m := pickStrictAlbumMatch([]SpotifyAlbum{mk("Dopesmoker", "Sleep", "2004", cover)}, "Sleep", "Dopesmoker", spotifyIntPtr(2003))
		assert.NotNil(t, m)
	})

	t.Run("release year set but candidate year unknown → match (no reject on missing)", func(t *testing.T) {
		m := pickStrictAlbumMatch([]SpotifyAlbum{mk("Dopesmoker", "Sleep", "", cover)}, "Sleep", "Dopesmoker", spotifyIntPtr(2003))
		assert.NotNil(t, m)
	})

	t.Run("qualifying candidate with no image → nil", func(t *testing.T) {
		m := pickStrictAlbumMatch([]SpotifyAlbum{mk("Dopesmoker", "Sleep", "2003", nil)}, "Sleep", "Dopesmoker", nil)
		assert.Nil(t, m)
	})

	t.Run("multiple candidates → first qualifying wins", func(t *testing.T) {
		albums := []SpotifyAlbum{
			mk("Dopesmoker (Remastered)", "Sleep", "2012", cover), // name mismatch
			mk("Dopesmoker", "Sleep", "2003", cover),              // qualifies
		}
		m := pickStrictAlbumMatch(albums, "Sleep", "Dopesmoker", nil)
		if assert.NotNil(t, m) {
			assert.Equal(t, "Dopesmoker", m.Name)
		}
	})

	t.Run("multi-artist album, our artist is featured → still matches", func(t *testing.T) {
		alb := SpotifyAlbum{
			Name:    "Dopesmoker",
			Artists: []SpotifyAlbumArtistRef{{Name: "Sleep"}, {Name: "Guest"}},
			Images:  cover,
		}
		m := pickStrictAlbumMatch([]SpotifyAlbum{alb}, "Guest", "Dopesmoker", nil)
		assert.NotNil(t, m)
	})

	t.Run("empty title → nil", func(t *testing.T) {
		assert.Nil(t, pickStrictAlbumMatch([]SpotifyAlbum{mk("Dopesmoker", "Sleep", "2003", cover)}, "Sleep", "", nil))
	})

	t.Run("no candidates → nil", func(t *testing.T) {
		assert.Nil(t, pickStrictAlbumMatch(nil, "Sleep", "Dopesmoker", nil))
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
		{"  https://open.spotify.com/artist/ABC123  ", "ABC123"}, // trimmed
		// Rejected: look-alike host (SSRF-style), wrong path, no scheme, junk.
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

func TestPrimaryArtistName(t *testing.T) {
	assert.Equal(t, "", primaryArtistName(nil))
}

func TestAbsInt(t *testing.T) {
	assert.Equal(t, 3, absInt(-3))
	assert.Equal(t, 3, absInt(3))
	assert.Equal(t, 0, absInt(0))
}
