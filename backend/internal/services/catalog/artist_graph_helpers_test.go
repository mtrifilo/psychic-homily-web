package catalog

import "testing"

// TestHasEmbeddableSpotify pins the "no dead markers" contract for the PSY-1379
// playable-node flag: the flag must only mark Spotify URLs the frontend
// MusicEmbed (parseSpotifyEmbed, lib/spotify.ts) can actually render — canonical
// open.spotify.com artist/album/track URLs with a 22-char base62 id.
func TestHasEmbeddableSpotify(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"canonical artist", "https://open.spotify.com/artist/3TVXtAsR1Inumwj472S9r4", true},
		{"album", "https://open.spotify.com/album/1DFixLWuPkv3KT3TnV35m3", true},
		{"track", "https://open.spotify.com/track/3n3Ppam7vgaVa1iaRUc9Lp", true},
		{"with ?si= share param", "https://open.spotify.com/artist/3TVXtAsR1Inumwj472S9r4?si=abc123", true},
		{"locale prefix", "https://open.spotify.com/intl-de/artist/3TVXtAsR1Inumwj472S9r4", true},
		{"trailing segment", "https://open.spotify.com/artist/3TVXtAsR1Inumwj472S9r4/about", true},
		{"scheme-less stored value", "open.spotify.com/artist/3TVXtAsR1Inumwj472S9r4", true},
		{"short id — unembeddable", "https://open.spotify.com/artist/short", false},
		{"wrong host", "https://evil.test/artist/3TVXtAsR1Inumwj472S9r4", false},
		{"playlist — not an embeddable kind", "https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M", false},
		{"empty", "", false},
		{"uri form not handled (stored values are http URLs)", "spotify:artist:3TVXtAsR1Inumwj472S9r4", false},
	}
	for _, c := range cases {
		if got := hasEmbeddableSpotify(c.in); got != c.want {
			t.Errorf("%s: hasEmbeddableSpotify(%q) = %v, want %v", c.name, c.in, got, c.want)
		}
	}
}
