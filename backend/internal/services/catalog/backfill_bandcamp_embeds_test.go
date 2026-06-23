package catalog

import (
	"testing"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// bcRel builds a Release with the given id/year and external links. Platform is
// intentionally varied across cases and ignored by the selection rule — the URL
// is the source of truth, not the label. (intPtr / stringPtr live in
// helpers_test.go.)
func bcRel(id uint, year *int, links ...catalogm.ReleaseExternalLink) catalogm.Release {
	return catalogm.Release{
		ID:            id,
		ReleaseYear:   year,
		ExternalLinks: links,
	}
}

func bcLink(id uint, platform, url string) catalogm.ReleaseExternalLink {
	return catalogm.ReleaseExternalLink{ID: id, Platform: platform, URL: url}
}

func TestSelectBandcampEmbedFromReleases(t *testing.T) {
	// vars (not consts) so the table can take their addresses for the *string
	// want field.
	var (
		albumA = "https://artificialgo.bandcamp.com/album/triple-ones"
		albumB = "https://artificialgo.bandcamp.com/album/second-record"
		trackA = "https://artificialgo.bandcamp.com/track/one-song"
	)

	tests := []struct {
		name     string
		releases []catalogm.Release
		want     *string // nil => expect no embed
	}{
		{
			name:     "no releases",
			releases: nil,
			want:     nil,
		},
		{
			name: "release with no external links",
			releases: []catalogm.Release{
				bcRel(1, intPtr(2020)),
			},
			want: nil,
		},
		{
			name: "release with only non-bandcamp links",
			releases: []catalogm.Release{
				bcRel(1, intPtr(2020),
					bcLink(10, "spotify", "https://open.spotify.com/album/abc"),
					bcLink(11, "bandcamp", "https://artificialgo.bandcamp.com"), // bare profile, not /album|/track
				),
			},
			want: nil,
		},
		{
			name: "single release single album link",
			releases: []catalogm.Release{
				bcRel(1, intPtr(2020), bcLink(10, "bandcamp", albumA)),
			},
			want: &albumA,
		},
		{
			name: "mislabeled platform is still matched (URL is source of truth)",
			releases: []catalogm.Release{
				bcRel(1, intPtr(2020), bcLink(10, "discogs", albumA)),
			},
			want: &albumA,
		},
		{
			name: "non-bandcamp URL labeled bandcamp is rejected",
			releases: []catalogm.Release{
				bcRel(1, intPtr(2020), bcLink(10, "bandcamp", "https://evil.test/album/x")),
			},
			want: nil,
		},
		{
			name: "most recent year wins across releases",
			releases: []catalogm.Release{
				bcRel(1, intPtr(2018), bcLink(10, "bandcamp", albumA)),
				bcRel(2, intPtr(2022), bcLink(20, "bandcamp", albumB)),
			},
			want: &albumB,
		},
		{
			name: "null year sorts last so dated release wins",
			releases: []catalogm.Release{
				bcRel(1, nil, bcLink(10, "bandcamp", albumA)),
				bcRel(2, intPtr(2010), bcLink(20, "bandcamp", albumB)),
			},
			want: &albumB,
		},
		{
			name: "all null years => lowest release id wins (deterministic)",
			releases: []catalogm.Release{
				bcRel(5, nil, bcLink(50, "bandcamp", albumB)),
				bcRel(2, nil, bcLink(20, "bandcamp", albumA)),
			},
			want: &albumA, // release 2 < release 5
		},
		{
			name: "same year: album beats track",
			releases: []catalogm.Release{
				bcRel(1, intPtr(2021), bcLink(10, "bandcamp", trackA)),
				bcRel(2, intPtr(2021), bcLink(20, "bandcamp", albumB)),
			},
			want: &albumB,
		},
		{
			name: "same year same kind: lowest release id wins",
			releases: []catalogm.Release{
				bcRel(7, intPtr(2021), bcLink(70, "bandcamp", albumB)),
				bcRel(3, intPtr(2021), bcLink(30, "bandcamp", albumA)),
			},
			want: &albumA, // release 3 < release 7
		},
		{
			name: "within one release: album beats track regardless of link id order",
			releases: []catalogm.Release{
				bcRel(1, intPtr(2021),
					bcLink(10, "bandcamp", trackA), // lower link id, but a track
					bcLink(11, "bandcamp", albumA), // higher link id, but an album
				),
			},
			want: &albumA,
		},
		{
			name: "track-only artist still resolves to the track",
			releases: []catalogm.Release{
				bcRel(1, intPtr(2021), bcLink(10, "bandcamp", trackA)),
			},
			want: &trackA,
		},
		{
			// A /track/ URL with "/album/" in its query string must NOT be
			// classified as an album, so a real /album/ in another release
			// still wins the album-over-track tie-break.
			name: "track url with /album/ in query string is not an album",
			releases: []catalogm.Release{
				bcRel(1, intPtr(2021), bcLink(10, "bandcamp", "https://artificialgo.bandcamp.com/track/song?from=/album/x")),
				bcRel(2, intPtr(2021), bcLink(20, "bandcamp", albumB)),
			},
			want: &albumB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectBandcampEmbedFromReleases(tt.releases)
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("expected nil, got %q", *got)
			case tt.want != nil && got == nil:
				t.Fatalf("expected %q, got nil", *tt.want)
			case tt.want != nil && got != nil && *got != *tt.want:
				t.Fatalf("expected %q, got %q", *tt.want, *got)
			}
		})
	}
}

func TestManualEmbedSourceIfSet(t *testing.T) {
	empty := ""
	blank := "   "
	url := "https://artificialgo.bandcamp.com/album/triple-ones"

	tests := []struct {
		name string
		in   *string
		want *string // nil or "manual"
	}{
		{"nil pointer", nil, nil},
		{"empty string", &empty, nil},
		{"whitespace only", &blank, nil},
		{"set value", &url, stringPtr(catalogm.BandcampEmbedSourceManual)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := manualEmbedSourceIfSet(tt.in)
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("expected nil, got %q", *got)
			case tt.want != nil && got == nil:
				t.Fatalf("expected %q, got nil", *tt.want)
			case tt.want != nil && got != nil && *got != *tt.want:
				t.Fatalf("expected %q, got %q", *tt.want, *got)
			}
		})
	}
}
