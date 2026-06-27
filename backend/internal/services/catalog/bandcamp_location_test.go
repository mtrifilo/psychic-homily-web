package catalog

import (
	"context"
	"net/http"
	"testing"
)

// TestExtractBandcampLocation pins the pure parse of the profile-header location
// element across the real-world shapes (full state, country, extra classes, HTML
// entities) and confirms a SHOW-LISTING venue (JSON-LD MusicVenue, no
// `location secondaryText` class) is never mistaken for the band's home.
func TestExtractBandcampLocation(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		want   string
		wantOK bool
	}{
		{
			name:   "us city + full state",
			body:   `<div><span class="location secondaryText">Phoenix, Arizona</span></div>`,
			want:   "Phoenix, Arizona",
			wantOK: true,
		},
		{
			name:   "international city + country",
			body:   `<span class="location secondaryText">Tokyo, Japan</span>`,
			want:   "Tokyo, Japan",
			wantOK: true,
		},
		{
			name:   "extra classes around the marker",
			body:   `<p class="foo location secondaryText bar" data-x="1">Brooklyn, New York</p>`,
			want:   "Brooklyn, New York",
			wantOK: true,
		},
		{
			name:   "html entities are unescaped",
			body:   `<span class="location secondaryText">Montr&eacute;al, Quebec</span>`,
			want:   "Montréal, Quebec",
			wantOK: true,
		},
		{
			name:   "no location element",
			body:   `<html><body><h1>A Band</h1></body></html>`,
			wantOK: false,
		},
		{
			name:   "empty location element",
			body:   `<span class="location secondaryText">   </span>`,
			wantOK: false,
		},
		{
			name:   "show-listing MusicVenue json-ld is not the band home",
			body:   `<script type="application/ld+json">{"location":{"@type":"MusicVenue","name":"The Rebel Lounge"}}</script>`,
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractBandcampLocation(tc.body)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestResolveProfileLocation exercises the fetch path end-to-end through the
// production SSRF-anchored client (reusing the canned round-tripper): a fetched
// profile page yields its location, and an unfetchable host fills nothing.
func TestResolveProfileLocation(t *testing.T) {
	t.Run("fetches and extracts the band location", func(t *testing.T) {
		rt := &cannedRoundTripper{byHost: map[string]cannedResponse{
			"band.bandcamp.com": {
				status: http.StatusOK,
				body:   `<header><h2 class="band-name">Band</h2><span class="location secondaryText">Phoenix, Arizona</span></header>`,
			},
		}}
		r := newCannedResolver(rt)
		got, ok := r.ResolveProfileLocation(context.Background(), "https://band.bandcamp.com/")
		if !ok {
			t.Fatal("expected ok")
		}
		if got != "Phoenix, Arizona" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("non-bandcamp host fills nothing", func(t *testing.T) {
		r := newCannedResolver(&cannedRoundTripper{byHost: map[string]cannedResponse{}})
		if _, ok := r.ResolveProfileLocation(context.Background(), "https://evil.test/"); ok {
			t.Fatal("expected ok=false for a disallowed host")
		}
	})

	t.Run("page without location fills nothing", func(t *testing.T) {
		rt := &cannedRoundTripper{byHost: map[string]cannedResponse{
			"band.bandcamp.com": {status: http.StatusOK, body: `<html><body>no location here</body></html>`},
		}}
		r := newCannedResolver(rt)
		if _, ok := r.ResolveProfileLocation(context.Background(), "https://band.bandcamp.com/"); ok {
			t.Fatal("expected ok=false")
		}
	})
}
