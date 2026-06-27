package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Artist-photo enrichment from Wikimedia Commons via MusicBrainz → Wikidata
// (PSY-1232). Populates artist photos for artists that have none, storing only a
// REFERENCE to the Commons-hosted image (URL + provider id + linkback + CC
// license + author) — never the bytes (PSY-1175 D1/D3).
//
// This is the Spotify-free, durable artist-photo path: MusicBrainz artist →
// Wikidata P18 (image) → Commons file. CC licenses are explicit + irrevocable,
// so the photo is hotlinked references-only with CC attribution — the same
// low-risk hotlink tier as the CAA cover enricher (PSY-1216), no DMCA §512.
//
// Match policy = ID-ANCHORED. A wrong-artist photo is worse than none, and
// common names ("Crush", "Muscle") collide across distinct MusicBrainz artists.
// So:
//   - a UNIQUE exact-name MusicBrainz match is used directly (low collision risk);
//   - when MULTIPLE artists share the exact name, the match must be disambiguated
//     by a SHARED external link — one of our artist's curated links (Spotify /
//     Bandcamp / SoundCloud / website) appearing in the MusicBrainz artist's
//     url-relations — otherwise the artist is skipped.
//
// Only freely-licensed images are stored (the Commons client gates the license);
// every stored URL is validated before persisting (https upload.wikimedia.org
// image; commons.wikimedia.org linkback) per the validate-on-write contract.

const (
	commonsImageSource = "commons"
	// commonsURLLookupCap bounds how many same-name MusicBrainz candidates we fetch
	// url-relations for per artist — each is a rate-limited MB call, and >5 distinct
	// artists sharing an exact name is rare.
	commonsURLLookupCap = 5
)

// MBArtistCandidate is the catalog-local view of a MusicBrainz artist search
// result, so this package needn't depend on the pipeline package. The cmd adapts
// the shared pipeline.MusicBrainzClient to the interfaces below.
type MBArtistCandidate struct {
	MBID string
	Name string
}

// musicBrainzArtistAPI is the slice of MB artist operations the enricher needs.
type musicBrainzArtistAPI interface {
	SearchArtistCandidates(ctx context.Context, name string) ([]MBArtistCandidate, error)
	LookupArtistURLs(ctx context.Context, mbid string) ([]string, error)
}

// wikidataImageAPI resolves a Wikidata entity's P18 image filename.
type wikidataImageAPI interface {
	ImageFilename(ctx context.Context, qid string) (string, error)
}

// commonsImageAPI resolves a Commons filename to a hotlinkable, freely-licensed image.
type commonsImageAPI interface {
	ImageInfo(ctx context.Context, filename string) (*CommonsImage, error)
}

// CommonsEnrichOptions configures a backfill run.
type CommonsEnrichOptions struct {
	DryRun bool
	Limit  int
}

// CommonsEnrichReport summarizes a backfill run.
//   - Scanned: artists with no photo loaded by the query.
//   - Matched: an ID-anchored artist resolved to a stored freely-licensed photo
//     that passed URL validation.
//   - Updated: written (always 0 on a dry run).
//   - Skipped: no confident MB match / no Wikidata / no P18 image / no free image /
//     a validation failure.
//   - Errors: API/DB failures; the artist is left untouched, safe to retry on a re-run.
type CommonsEnrichReport struct {
	DryRun bool

	ArtistsScanned int
	ArtistsMatched int
	ArtistsUpdated int
	ArtistsSkipped int
	ArtistErrors   int
}

// BackfillCommonsPhotos enriches artist photos that are currently empty from
// Wikimedia Commons. References only, never bytes. Idempotent: only photo-less
// artists are considered, so a re-run after a live pass reports zero updates, and
// an errored artist is safe to retry by re-running.
func BackfillCommonsPhotos(
	ctx context.Context,
	db *gorm.DB,
	mb musicBrainzArtistAPI,
	wd wikidataImageAPI,
	commons commonsImageAPI,
	opts CommonsEnrichOptions,
) (*CommonsEnrichReport, error) {
	report := &CommonsEnrichReport{DryRun: opts.DryRun}

	var artists []catalogm.Artist
	q := db.WithContext(ctx).
		Where("image_url IS NULL OR image_url = ''").
		Order("id")
	if opts.Limit > 0 {
		q = q.Limit(opts.Limit)
	}
	if err := q.Find(&artists).Error; err != nil {
		return report, fmt.Errorf("loading artists: %w", err)
	}

	for i := range artists {
		ar := &artists[i]
		report.ArtistsScanned++

		if strings.TrimSpace(ar.Name) == "" {
			report.ArtistsSkipped++
			continue
		}

		img, err := resolveCommonsPhoto(ctx, mb, wd, commons, ar)
		if err != nil {
			report.ArtistErrors++
			slog.Warn("commons-enrich: lookup failed",
				"artist_id", ar.ID, "name", ar.Name, "error", err)
			continue
		}
		if img == nil {
			report.ArtistsSkipped++
			slog.Info("commons-enrich: no confident Commons photo; skipping",
				"artist_id", ar.ID, "name", ar.Name)
			continue
		}

		// Validate-on-write (PSY-525/747/1113): image_url renders in <img src> and
		// image_source_url as an attribution <a href>, so pin the image to the
		// Commons CDN host and the linkback to the Commons site. License was already
		// gated to a reusable one by the Commons client.
		if !validCommonsImageURL(img.ImageURL) || !isCommonsWebURL(img.DescriptionURL) || strings.TrimSpace(img.License) == "" {
			report.ArtistsSkipped++
			slog.Warn("commons-enrich: photo failed validation; skipping",
				"artist_id", ar.ID, "image_url", img.ImageURL, "source_url", img.DescriptionURL, "license", img.License)
			continue
		}
		report.ArtistsMatched++

		slog.Info("commons-enrich: artist photo match",
			"artist_id", ar.ID, "name", ar.Name, "image_url", img.ImageURL,
			"license", img.License, "author", img.Author, "dry_run", opts.DryRun)

		if opts.DryRun {
			continue
		}

		updates := map[string]any{
			"image_url":        img.ImageURL,
			"image_source":     commonsImageSource,
			"image_source_url": img.DescriptionURL,
			"image_license":    img.License,
			"image_author":     nilIfEmpty(img.Author),
		}
		if err := db.WithContext(ctx).Model(&catalogm.Artist{}).Where("id = ?", ar.ID).Updates(updates).Error; err != nil {
			report.ArtistErrors++
			slog.Warn("commons-enrich: failed to update artist", "artist_id", ar.ID, "error", err)
			continue
		}
		report.ArtistsUpdated++
	}

	return report, nil
}

// resolveCommonsPhoto resolves an artist to a freely-licensed Commons photo via an
// ID-anchored MusicBrainz match → Wikidata P18 → Commons. Returns (nil, nil) when
// no confident photo is found. A transport error is returned so the caller counts
// it + retries on a later run.
func resolveCommonsPhoto(
	ctx context.Context,
	mb musicBrainzArtistAPI,
	wd wikidataImageAPI,
	commons commonsImageAPI,
	ar *catalogm.Artist,
) (*CommonsImage, error) {
	wantName := normalizeForMatch(ar.Name)
	if wantName == "" {
		return nil, nil
	}

	cands, err := mb.SearchArtistCandidates(ctx, ar.Name)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz search: %w", err)
	}

	// Keep only exact normalized-name matches (the discovery exact-name gate,
	// PSY-1191 — never trust MB's relevance score for identity).
	var exact []MBArtistCandidate
	for _, c := range cands {
		if strings.TrimSpace(c.MBID) != "" && normalizeForMatch(c.Name) == wantName {
			exact = append(exact, c)
		}
	}
	if len(exact) == 0 {
		return nil, nil
	}

	// Our artist's curated external links — the anchors that disambiguate same-name
	// MB artists.
	anchors := artistLinkKeys(ar)

	qid := ""
	if len(exact) == 1 {
		// Unique exact-name match — low collision risk, use it directly.
		urls, err := mb.LookupArtistURLs(ctx, exact[0].MBID)
		if err != nil {
			return nil, fmt.Errorf("musicbrainz url-rels: %w", err)
		}
		qid = extractWikidataQID(urls)
	} else {
		// Multiple same-name artists — require a SHARED LINK to disambiguate.
		var anchoredQIDs []string
		for i, c := range exact {
			if i >= commonsURLLookupCap {
				slog.Info("commons-enrich: many same-name MB artists; capping url-rels lookups",
					"artist_id", ar.ID, "name", ar.Name, "exact", len(exact), "cap", commonsURLLookupCap)
				break
			}
			urls, err := mb.LookupArtistURLs(ctx, c.MBID)
			if err != nil {
				return nil, fmt.Errorf("musicbrainz url-rels: %w", err)
			}
			if len(anchors) > 0 && urlsShareLink(urls, anchors) {
				if q := extractWikidataQID(urls); q != "" {
					anchoredQIDs = append(anchoredQIDs, q)
				}
			}
		}
		// Exactly one anchored artist with a Wikidata id resolves the ambiguity;
		// zero (no shared link) or conflicting anchors → skip.
		if dedupOne(anchoredQIDs) {
			qid = anchoredQIDs[0]
		}
	}

	if qid == "" {
		return nil, nil
	}

	fname, err := wd.ImageFilename(ctx, qid)
	if err != nil {
		return nil, fmt.Errorf("wikidata: %w", err)
	}
	if fname == "" {
		return nil, nil
	}

	img, err := commons.ImageInfo(ctx, fname)
	if err != nil {
		return nil, fmt.Errorf("commons: %w", err)
	}
	return img, nil // nil when the file is missing or not freely licensed
}

// =============================================================================
// Helpers (pure — unit-tested without HTTP or DB)
// =============================================================================

var wikidataQIDPattern = regexp.MustCompile(`wikidata\.org/(?:wiki|entity)/(Q\d+)`)

// extractWikidataQID returns the first Wikidata entity id among a set of artist
// url-relations, or "".
func extractWikidataQID(urls []string) string {
	for _, u := range urls {
		if m := wikidataQIDPattern.FindStringSubmatch(u); m != nil {
			return m[1]
		}
	}
	return ""
}

// artistLinkKeys returns the normalized identity keys of an artist's curated
// external links (Spotify / Bandcamp / SoundCloud / website) — the anchors used
// to disambiguate same-name MusicBrainz artists.
func artistLinkKeys(ar *catalogm.Artist) []string {
	var keys []string
	for _, p := range []*string{ar.Social.Spotify, ar.Social.Bandcamp, ar.Social.SoundCloud, ar.Social.Website} {
		if p == nil {
			continue
		}
		if k := normalizeLink(*p); k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}

// urlsShareLink reports whether any of the url-relations normalizes to one of the
// anchor keys. The comparison is on the full host+path identity (not host alone),
// so two distinct artists who merely both have an open.spotify.com link don't
// falsely anchor — only a matching /artist/{id} does.
func urlsShareLink(urls, anchorKeys []string) bool {
	set := make(map[string]struct{}, len(anchorKeys))
	for _, k := range anchorKeys {
		set[k] = struct{}{}
	}
	for _, u := range urls {
		if k := normalizeLink(u); k != "" {
			if _, ok := set[k]; ok {
				return true
			}
		}
	}
	return false
}

// normalizeLink reduces a URL to a comparable identity key: lowercased host (less
// a leading "www.") + lowercased path (trailing slash trimmed), scheme dropped. So
// "https://Artist.bandcamp.com/" and "http://artist.bandcamp.com" both key to
// "artist.bandcamp.com", and two Spotify links match only on the same /artist/id.
func normalizeLink(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	path := strings.TrimRight(strings.ToLower(u.Path), "/")
	return host + path
}

// validCommonsImageURL reports whether raw is an https image on the Commons CDN
// host. Gates image_url before persisting (it renders in <img src>).
func validCommonsImageURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return u.Scheme == "https" && strings.ToLower(u.Hostname()) == commonsImageHost
}

// isCommonsWebURL reports whether raw is an https commons.wikimedia.org URL. Gates
// the image_source_url linkback (the Commons file page) before persisting.
func isCommonsWebURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return u.Scheme == "https" && strings.ToLower(u.Hostname()) == "commons.wikimedia.org"
}

// dedupOne reports whether the values collapse to exactly one distinct non-empty
// string — used to confirm a SINGLE anchored Wikidata id resolved the ambiguity.
func dedupOne(vals []string) bool {
	seen := ""
	for _, v := range vals {
		if v == "" {
			continue
		}
		if seen == "" {
			seen = v
		} else if v != seen {
			return false
		}
	}
	return seen != ""
}

// nilIfEmpty returns nil for a blank string (→ SQL NULL in a GORM updates map),
// else the trimmed string. image_author is optional (public-domain files often
// have no author).
func nilIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
