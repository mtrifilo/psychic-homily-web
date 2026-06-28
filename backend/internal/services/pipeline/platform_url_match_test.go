package pipeline

import "testing"

func TestSamePlatformArtistURL(t *testing.T) {
	const a = "https://open.spotify.com/artist/3WrFJ7ztbogyGnTHbHJFl2"
	tests := []struct {
		name string
		x, y string
		want bool
	}{
		{"identical spotify", a, a, true},
		{"spotify trailing slash + query canonicalize equal", a, a + "/?si=abc", true},
		{"spotify http vs https canonicalize equal", "http://open.spotify.com/artist/3WrFJ7ztbogyGnTHbHJFl2", a, true},
		{"different spotify artists", a, "https://open.spotify.com/artist/0000000000000000000000", false},
		{"bandcamp apex matches itself", "https://band.bandcamp.com", "https://band.bandcamp.com/", true},
		{"spotify vs bandcamp differ", a, "https://band.bandcamp.com", false},
		{"non-platform url is never a match", "https://example.com/x", "https://example.com/x", false},
		{"spotify album (not artist) is not a match", "https://open.spotify.com/album/123", a, false},
		{"empty is not a match", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SamePlatformArtistURL(tt.x, tt.y); got != tt.want {
				t.Errorf("SamePlatformArtistURL(%q,%q)=%v, want %v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}
