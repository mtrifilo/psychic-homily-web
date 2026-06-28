// Package mbadapter bridges the shared pipeline.MusicBrainzClient to the catalog
// image-enricher interfaces (catalog's musicBrainzArtistAPI / musicBrainzReleaseSearcher),
// producing catalog.MBArtistCandidate / catalog.MBReleaseGroupCandidate.
//
// It is the ONE home for that bridge (PSY-1248): the adapters + their mapping
// helpers were previously copy-pasted in three near-identical places (the
// imageenrich sweep + both cmd/backfill-* CLIs), a drift trap if the catalog
// candidate types ever gain a field.
//
// It lives in its own leaf package — NOT in catalog, where the candidate types
// live — because catalog must not import pipeline: pipeline's (internal) tests
// import catalog, so a catalog→pipeline edge would cycle (the constraint PSY-1246
// was relocated to honor). A dedicated leaf also keeps the dependency direction
// right: the sweep service AND the backfill CLIs depend DOWN on this adapter, none
// of them on each other.
package mbadapter

import (
	"context"

	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/pipeline"
)

// ArtistAdapter satisfies catalog's musicBrainzArtistAPI using the shared MB client.
type ArtistAdapter struct {
	client *pipeline.MusicBrainzClient
}

// NewArtistAdapter wraps the shared MusicBrainz client for artist enrichment.
func NewArtistAdapter(client *pipeline.MusicBrainzClient) ArtistAdapter {
	return ArtistAdapter{client: client}
}

func (a ArtistAdapter) SearchArtistCandidates(ctx context.Context, name string) ([]catalog.MBArtistCandidate, error) {
	raw, err := a.client.SearchArtistCandidates(ctx, name)
	if err != nil {
		return nil, err
	}
	return ToMBArtistCandidates(raw), nil
}

func (a ArtistAdapter) LookupArtistURLs(ctx context.Context, mbid string) ([]string, error) {
	rels, err := a.client.LookupArtistURLRelations(ctx, mbid)
	if err != nil {
		return nil, err
	}
	return ToURLResources(rels), nil
}

// ReleaseAdapter satisfies catalog's musicBrainzReleaseSearcher using the shared MB client.
type ReleaseAdapter struct {
	client *pipeline.MusicBrainzClient
}

// NewReleaseAdapter wraps the shared MusicBrainz client for release-cover enrichment.
func NewReleaseAdapter(client *pipeline.MusicBrainzClient) ReleaseAdapter {
	return ReleaseAdapter{client: client}
}

func (a ReleaseAdapter) SearchReleaseGroups(ctx context.Context, artist, title string, limit int) ([]catalog.MBReleaseGroupCandidate, error) {
	raw, err := a.client.SearchReleaseGroups(ctx, artist, title, limit)
	if err != nil {
		return nil, err
	}
	out := make([]catalog.MBReleaseGroupCandidate, 0, len(raw))
	for _, rg := range raw {
		out = append(out, catalog.MBReleaseGroupCandidate{
			MBID:             rg.ID,
			Title:            rg.Title,
			ArtistNames:      FlattenArtistNames(rg.ArtistCredit),
			FirstReleaseDate: rg.FirstReleaseDate,
		})
	}
	return out, nil
}

// ToMBArtistCandidates maps MusicBrainz search results to the catalog candidate type.
func ToMBArtistCandidates(raw []pipeline.MBArtistResult) []catalog.MBArtistCandidate {
	out := make([]catalog.MBArtistCandidate, 0, len(raw))
	for _, r := range raw {
		out = append(out, catalog.MBArtistCandidate{MBID: r.ID, Name: r.Name})
	}
	return out
}

// ToURLResources flattens MusicBrainz url-relations to their resource URLs,
// dropping empty entries.
func ToURLResources(rels []pipeline.MBURLRelation) []string {
	urls := make([]string, 0, len(rels))
	for _, r := range rels {
		if r.URL.Resource != "" {
			urls = append(urls, r.URL.Resource)
		}
	}
	return urls
}

// FlattenArtistNames collects the credited + canonical artist names from a
// release-group's artist credit, giving the strict matcher both forms to match
// against. The credited name is the form printed on the release (may be an alias /
// "feat." rendering); the canonical name is the artist's MusicBrainz name. Empty
// names are skipped, and the canonical is omitted when it equals the credited.
func FlattenArtistNames(credits []pipeline.MBArtistCredit) []string {
	names := make([]string, 0, len(credits)*2)
	for _, ac := range credits {
		if ac.Name != "" {
			names = append(names, ac.Name)
		}
		if ac.Artist.Name != "" && ac.Artist.Name != ac.Name {
			names = append(names, ac.Artist.Name)
		}
	}
	return names
}
