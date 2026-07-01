package enrich

import (
	"testing"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/pipeline"
)

func mbURLRel(t, resource string) pipeline.MBURLRelation {
	r := pipeline.MBURLRelation{Type: t}
	r.URL.Resource = resource
	return r
}

func TestLinksUpdateFromRels_FillWhenEmpty(t *testing.T) {
	spotifyExisting := "https://open.spotify.com/artist/existing"
	a := &catalogm.Artist{
		Name:                "Band",
		MusicBrainzArtistID: strptr("11111111-1111-1111-1111-111111111111"),
		Social: catalogm.Social{
			Spotify: &spotifyExisting,
		},
	}
	rels := []pipeline.MBURLRelation{
		mbURLRel("free streaming", "https://open.spotify.com/artist/other"),
		mbURLRel("bandcamp", "https://band.bandcamp.com/"),
		mbURLRel("official homepage", "https://band.example.com"),
	}
	req := linksUpdateFromRels(a, rels)
	if req == nil {
		t.Fatal("expected update request")
	}
	if req.Spotify != nil {
		t.Error("spotify must not be overwritten")
	}
	if req.Bandcamp == nil || *req.Bandcamp != "https://band.bandcamp.com" {
		t.Errorf("bandcamp = %v, want canonical bandcamp URL", req.Bandcamp)
	}
	if req.Website == nil || *req.Website != "https://band.example.com" {
		t.Errorf("website = %v", req.Website)
	}
}

func TestLinksUpdateFromRels_SkipsNonOfficialWebsite(t *testing.T) {
	a := &catalogm.Artist{Name: "Band"}
	rels := []pipeline.MBURLRelation{
		mbURLRel("social network", "https://example.com"),
	}
	req := linksUpdateFromRels(a, rels)
	if req != nil {
		t.Fatalf("non-official homepage must not fill website: %+v", req)
	}
}

func TestClassifyOfficialHomepage_RejectsPlatformHosts(t *testing.T) {
	_, ok := classifyOfficialHomepage(mbURLRel("official homepage", "https://evil.bandcamp.com/"))
	if ok {
		t.Error("bandcamp host must not become website")
	}
}
